// Package preview shows each side's application: http://<side>-<id>.localhost:<APP_PORT>/.
//
// With no preview command in the profile, the files the side produced (its saved workspace.tar)
// are served as a static site. With a command, a container is started from the side's result
// image on the agent network and api proxies to it. Each side gets its own origin (a *.localhost
// subdomain, which browsers resolve to this machine), so applications that use absolute paths
// work, and a preview cannot read or drive ai-compare's own pages and API.
package preview

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// Target is what a side's preview runs from.
type Target struct {
	// Tar is the side's saved files (workspace.tar).
	Tar string
	// Image is the side's result image, for command previews.
	Image string
	// Command and Port come from the profile; an empty command means a static preview.
	Command string
	Port    int
}

// Source finds a side's preview target (the comparison service).
type Source interface {
	PreviewTarget(id, side string) (Target, error)
}

type Status struct {
	// Status is "stopped", "starting", "running" or "error".
	Status string
	Kind   string
	URL    string
	Error  string
	Logs   string
}

type Options struct {
	Docker       *client.Client
	Source       Source
	AgentNetwork string
	// WorkDir holds the extracted files of static previews.
	WorkDir string
	AppPort int
	Log     *slog.Logger
}

// IdleTimeout stops command previews nobody has used for a while.
const IdleTimeout = 30 * time.Minute

type instance struct {
	status      Status
	containerID string
	dir         string
	proxy       *httputil.ReverseProxy
	lastUsed    time.Time
}

type Manager struct {
	opts Options
	mu   sync.Mutex
	live map[string]*instance
}

func New(opts Options) *Manager {
	m := &Manager{opts: opts, live: map[string]*instance{}}
	go m.reap()
	return m
}

func key(id, side string) string { return strings.ToLower(side) + "-" + id }

// URL is where a side's preview answers.
func (m *Manager) URL(id, side string) string {
	return fmt.Sprintf("http://%s.localhost:%d/", key(id, side), m.opts.AppPort)
}

func (m *Manager) Get(id, side string) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if in := m.live[key(id, side)]; in != nil {
		return in.status
	}
	return Status{Status: "stopped", URL: m.URL(id, side)}
}

// Start serves a side's files, or starts its preview container and returns while it boots.
func (m *Manager) Start(ctx context.Context, id, side string) (Status, error) {
	t, err := m.opts.Source.PreviewTarget(id, side)
	if err != nil {
		return Status{}, err
	}
	k := key(id, side)
	m.mu.Lock()
	if in := m.live[k]; in != nil && (in.status.Status == "running" || in.status.Status == "starting") {
		m.mu.Unlock()
		return in.status, nil
	}
	m.mu.Unlock()
	m.Stop(context.WithoutCancel(ctx), id, side)

	if strings.TrimSpace(t.Command) == "" {
		dir := filepath.Join(m.opts.WorkDir, "preview-"+k)
		if err := extract(t.Tar, dir); err != nil {
			return Status{}, fmt.Errorf("unpacking the side's files: %w", err)
		}
		st := Status{Status: "running", Kind: "static", URL: m.URL(id, side)}
		m.mu.Lock()
		m.live[k] = &instance{status: st, dir: dir, lastUsed: time.Now()}
		m.mu.Unlock()
		return st, nil
	}

	if t.Port <= 0 {
		return Status{}, fmt.Errorf("the profile has a preview command but no port")
	}
	if _, err := m.opts.Docker.ImageInspect(ctx, t.Image); err != nil {
		return Status{}, fmt.Errorf("the side's result image is gone (removed by retention?), so its application cannot run again; clear the preview command to serve its files instead")
	}
	st := Status{Status: "starting", Kind: "command", URL: m.URL(id, side)}
	in := &instance{status: st, lastUsed: time.Now()}
	m.mu.Lock()
	m.live[k] = in
	m.mu.Unlock()
	go m.run(id, side, t, in)
	return st, nil
}

func (m *Manager) run(id, side string, t Target, in *instance) {
	ctx := context.Background()
	fail := func(err error, logs string) {
		m.mu.Lock()
		in.status.Status, in.status.Error, in.status.Logs = "error", err.Error(), logs
		m.mu.Unlock()
		m.opts.Log.Warn("preview failed", "comparison", id, "side", side, "error", err)
	}
	created, err := m.opts.Docker.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      t.Image,
			Cmd:        []string{"sh", "-c", t.Command},
			WorkingDir: "/workspace",
			Env:        []string{fmt.Sprintf("PORT=%d", t.Port), "HOST=0.0.0.0"},
			Labels:     map[string]string{"ai-compare.comparison": id, "ai-compare.side": side, "ai-compare.role": "preview"},
		},
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode(m.opts.AgentNetwork),
			Resources:   container.Resources{NanoCPUs: 1e9, Memory: 2 << 30},
		},
	})
	if err != nil {
		fail(err, "")
		return
	}
	m.mu.Lock()
	in.containerID = created.ID
	m.mu.Unlock()
	if _, err := m.opts.Docker.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		fail(err, "")
		return
	}
	info, err := m.opts.Docker.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
	if err != nil || info.Container.NetworkSettings == nil {
		fail(fmt.Errorf("could not find the preview container's address"), "")
		return
	}
	ep := info.Container.NetworkSettings.Networks[m.opts.AgentNetwork]
	if ep == nil || !ep.IPAddress.IsValid() {
		fail(fmt.Errorf("the preview container has no address on %s", m.opts.AgentNetwork), "")
		return
	}
	addr := net.JoinHostPort(ep.IPAddress.String(), fmt.Sprint(t.Port))

	// Wait for the application to listen; dev servers can take a while to compile.
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			c.Close()
			break
		}
		if insp, err := m.opts.Docker.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{}); err == nil && insp.Container.State != nil && !insp.Container.State.Running {
			fail(fmt.Errorf("the preview command exited with code %d", insp.Container.State.ExitCode), m.logs(created.ID))
			return
		}
		if time.Now().After(deadline) {
			fail(fmt.Errorf("nothing answered on port %d after 2 minutes; the command must listen on 0.0.0.0:%d", t.Port, t.Port), m.logs(created.ID))
			return
		}
		m.mu.Lock()
		stopped := m.live[key(id, side)] != in
		m.mu.Unlock()
		if stopped {
			return
		}
		time.Sleep(time.Second)
	}
	target, _ := url.Parse("http://" + addr)
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.FlushInterval = -1
	m.mu.Lock()
	in.proxy = rp
	in.status.Status = "running"
	m.mu.Unlock()
}

func (m *Manager) logs(containerID string) string {
	rc, err := m.opts.Docker.ContainerLogs(context.Background(), containerID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: "60"})
	if err != nil {
		return ""
	}
	defer rc.Close()
	var b bytes.Buffer
	stdcopy.StdCopy(&b, &b, rc)
	return b.String()
}

// Stop ends a side's preview.
func (m *Manager) Stop(ctx context.Context, id, side string) {
	m.mu.Lock()
	in := m.live[key(id, side)]
	delete(m.live, key(id, side))
	m.mu.Unlock()
	if in != nil && in.containerID != "" {
		m.opts.Docker.ContainerRemove(ctx, in.containerID, client.ContainerRemoveOptions{Force: true})
	}
}

// RemoveLeftovers removes preview containers of a previous api process.
func (m *Manager) RemoveLeftovers(ctx context.Context) {
	res, err := m.opts.Docker.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{}.Add("label", "ai-compare.role=preview")})
	if err != nil {
		return
	}
	for _, c := range res.Items {
		m.opts.Docker.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true})
	}
}

func (m *Manager) reap() {
	for range time.Tick(time.Minute) {
		var idle [][2]string
		m.mu.Lock()
		for k, in := range m.live {
			if in.containerID != "" && time.Since(in.lastUsed) > IdleTimeout {
				side, id, _ := strings.Cut(k, "-")
				idle = append(idle, [2]string{id, side})
			}
		}
		m.mu.Unlock()
		for _, p := range idle {
			m.opts.Log.Info("stopping an idle preview", "comparison", p[0], "side", p[1])
			m.Stop(context.Background(), p[0], strings.ToUpper(p[1]))
		}
	}
}

var previewHost = regexp.MustCompile(`^([ab])-(r[0-9a-f]+)\.localhost(:\d+)?$`)

// Route sends requests for a preview host to the preview and everything else to next.
func (m *Manager) Route(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		match := previewHost.FindStringSubmatch(strings.ToLower(r.Host))
		if match == nil {
			next.ServeHTTP(w, r)
			return
		}
		k := match[1] + "-" + match[2]
		m.mu.Lock()
		in := m.live[k]
		if in != nil {
			in.lastUsed = time.Now()
		}
		var proxy *httputil.ReverseProxy
		var dir, status string
		if in != nil {
			proxy, dir, status = in.proxy, in.dir, in.status.Status
		}
		m.mu.Unlock()
		switch {
		case dir != "":
			http.FileServer(http.Dir(dir)).ServeHTTP(w, r)
		case proxy != nil:
			proxy.ServeHTTP(w, r)
		default:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			if status == "" {
				status = "stopped"
			}
			fmt.Fprintf(w, "This preview is %s. Start it from the Preview tab of comparison %s in ai-compare.\n", status, match[2])
		}
	})
}

// extract unpacks a tar into dir, replacing what was there and refusing paths that leave it.
func extract(tarPath, dir string) error {
	f, err := os.Open(tarPath)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("the side has no saved files")
	}
	if err != nil {
		return err
	}
	defer f.Close()
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(h.Name, "./")))
		if name == "." || strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			continue
		}
		target := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, io.LimitReader(tr, 512<<20))
			out.Close()
			if err != nil {
				return err
			}
		}
	}
}
