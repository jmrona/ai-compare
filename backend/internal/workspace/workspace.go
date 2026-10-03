// Package workspace prepares what each side of a comparison runs on: it copies the user's
// project into the staging volume (through a helper container that mounts the host path
// read-only) and builds the side image from that copy.
//
// api talks to the host's Docker daemon through the mounted socket, so every container it
// creates is a sibling, and bind-mount paths are resolved by the daemon on the host. That is
// what lets api mount C:\... or /Users/... even though api itself cannot see those paths.
package workspace

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

//go:embed copier/Dockerfile copier/copy-project.sh copier/inspect-project.sh
var copierFiles embed.FS

const labelPrefix = "ai-compare."

type Options struct {
	// StagingVolume is the Docker volume shared with api, e.g. "ai-compare_staging".
	StagingVolume string
	// StagingDir is where api sees that volume, e.g. "/data/staging".
	StagingDir string
	Log        *slog.Logger
}

type Service struct {
	cli  *client.Client
	opts Options
}

func New(cli *client.Client, opts Options) *Service {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	return &Service{cli: cli, opts: opts}
}

// NewDockerClient connects to the daemon given by DOCKER_HOST, or the default socket.
func NewDockerClient() (*client.Client, error) {
	return client.New(client.FromEnv, client.WithAPIVersionNegotiation())
}

/* ── Copying the project ────────────────────────────────── */

type CopyResult struct {
	// Mode is "git" (tracked + non-ignored files) or "plain" (everything minus common build folders).
	Mode      string `json:"mode"`
	Files     int    `json:"files"`
	Kilobytes int    `json:"kilobytes"`
	// EnvFilesSkipped counts .env files left out of the copy (templates are kept).
	EnvFilesSkipped int           `json:"envFilesSkipped"`
	Took            time.Duration `json:"-"`
}

var (
	windowsPath = regexp.MustCompile(`^[a-zA-Z]:[\\/]`)
	// Errors the daemon returns when it cannot bind-mount a host path.
	mountErrors = []string{"mounts denied", "bind source path does not exist", "invalid mount config", "not a directory", "no such file or directory"}
)

var (
	// ErrPathNotShared means Docker cannot see the folder the user gave.
	ErrPathNotShared = errors.New("docker cannot access this folder")
	// ErrPathNotFound means the path does not exist or is not a folder.
	ErrPathNotFound = errors.New("the project folder does not exist")
)

// splitHostPath separates an absolute host path into the top-level folder to mount and the
// rest: C:\Users\me\app → (C:\, Users/me/app); /Users/me/app → (/Users, me/app).
func splitHostPath(p string) (anchor, rest string, err error) {
	if windowsPath.MatchString(p) {
		rest = strings.Trim(strings.ReplaceAll(p[3:], `\`, "/"), "/")
		return p[:2] + `\`, rest, nil
	}
	if !strings.HasPrefix(p, "/") {
		return "", "", fmt.Errorf("the project path must be absolute, got %q", p)
	}
	parts := strings.SplitN(strings.Trim(p, "/"), "/", 2)
	if parts[0] == "" {
		return "", "", fmt.Errorf("the project path cannot be the root folder")
	}
	anchor = "/" + parts[0]
	if len(parts) == 2 {
		rest = strings.Trim(parts[1], "/")
	}
	return anchor, rest, nil
}

// CopyProject copies hostPath into the staging volume under id/project.
func (s *Service) CopyProject(ctx context.Context, hostPath, id string) (CopyResult, error) {
	start := time.Now()
	stdout, err := s.runHelper(ctx, hostPath, "copy-project", []string{id}, true, id)
	if err != nil {
		return CopyResult{}, err
	}
	var res CopyResult
	if err := json.Unmarshal([]byte(lastLine(stdout)), &res); err != nil {
		return CopyResult{}, fmt.Errorf("unexpected output from the copy container: %q", stdout)
	}
	res.Took = time.Since(start)
	return res, nil
}

// Inspection describes a project before it is copied.
type Inspection struct {
	Git      bool     `json:"git"`
	Files    int      `json:"files"`
	Bytes    int64    `json:"bytes"`
	Harness  []string `json:"harness"`
	Markers  []string `json:"markers"`
	EnvFiles []string `json:"envFiles"`
}

// InspectProject lists what would be copied without copying it.
func (s *Service) InspectProject(ctx context.Context, hostPath string) (Inspection, error) {
	stdout, err := s.runHelper(ctx, hostPath, "inspect-project", nil, false, "inspect")
	if err != nil {
		return Inspection{}, err
	}
	var res Inspection
	if err := json.Unmarshal([]byte(lastLine(stdout)), &res); err != nil {
		return Inspection{}, fmt.Errorf("unexpected output from the inspect container: %q", stdout)
	}
	return res, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// runHelper runs a script of the helper image with the top-level folder of hostPath mounted
// read-only at /host. The script gets the path inside it after args.
func (s *Service) runHelper(ctx context.Context, hostPath, script string, args []string, withStaging bool, label string) (string, error) {
	anchor, rest, err := splitHostPath(hostPath)
	if err != nil {
		return "", err
	}
	image, err := s.ensureCopierImage(ctx)
	if err != nil {
		return "", err
	}
	mounts := []mount.Mount{{Type: mount.TypeBind, Source: anchor, Target: "/host", ReadOnly: true}}
	if withStaging {
		mounts = append(mounts, mount.Mount{Type: mount.TypeVolume, Source: s.opts.StagingVolume, Target: "/staging"})
	}
	created, err := s.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      image,
			Entrypoint: []string{"/usr/local/bin/" + script},
			Cmd:        append(append([]string{}, args...), rest),
			Labels:     map[string]string{labelPrefix + "comparison": label, labelPrefix + "role": script},
		},
		HostConfig: &container.HostConfig{Mounts: mounts},
	})
	if err != nil {
		return "", mountError(anchor, err)
	}
	defer s.cli.ContainerRemove(context.WithoutCancel(ctx), created.ID, client.ContainerRemoveOptions{Force: true})

	if _, err := s.cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return "", mountError(anchor, err)
	}
	wait := s.cli.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	var exitCode int64
	select {
	case res := <-wait.Result:
		exitCode = res.StatusCode
	case err := <-wait.Error:
		return "", fmt.Errorf("waiting for the %s container: %w", script, err)
	}
	stdout, stderr, err := s.logs(ctx, created.ID)
	if err != nil {
		return "", err
	}
	switch exitCode {
	case 0:
	case 3:
		return "", fmt.Errorf("%w: %s", ErrPathNotFound, hostPath)
	case 4:
		return "", fmt.Errorf("%w: %s cannot be read", ErrPathNotShared, hostPath)
	default:
		return "", fmt.Errorf("%s failed (exit %d): %s", script, exitCode, strings.TrimSpace(stderr))
	}
	if stderr = strings.TrimSpace(stderr); stderr != "" {
		s.opts.Log.Warn(script+" reported warnings", "stderr", stderr)
	}
	return stdout, nil
}

func mountError(hostPath string, err error) error {
	msg := strings.ToLower(err.Error())
	for _, m := range mountErrors {
		if strings.Contains(msg, m) {
			return fmt.Errorf("%w: %s. Check that the folder exists and that Docker is allowed to share it "+
				"(Docker Desktop: Settings → Resources → File sharing). Docker said: %v", ErrPathNotShared, hostPath, err)
		}
	}
	return fmt.Errorf("starting the copy container: %w", err)
}

func (s *Service) logs(ctx context.Context, containerID string) (stdout, stderr string, err error) {
	rc, err := s.cli.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", "", fmt.Errorf("reading the helper container output: %w", err)
	}
	defer rc.Close()
	var out, errOut bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &errOut, rc); err != nil {
		return "", "", fmt.Errorf("reading the copy container output: %w", err)
	}
	return out.String(), errOut.String(), nil
}

// ensureCopierImage builds the helper image the first time, tagged by the hash of its files
// so a change to the script produces a new image.
func (s *Service) ensureCopierImage(ctx context.Context) (string, error) {
	files := map[string][]byte{}
	sum := sha256.New()
	for _, name := range []string{"Dockerfile", "copy-project.sh", "inspect-project.sh"} {
		data, err := copierFiles.ReadFile("copier/" + name)
		if err != nil {
			return "", err
		}
		// Normalise line endings: a CRLF script would not run in Linux.
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		files[name] = data
		sum.Write(data)
	}
	tag := "ai-compare/copier:" + hex.EncodeToString(sum.Sum(nil))[:12]

	if _, err := s.cli.ImageInspect(ctx, tag); err == nil {
		return tag, nil
	}
	s.opts.Log.Info("building the copy helper image", "tag", tag)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), ModTime: time.Unix(0, 0)}); err != nil {
			return "", err
		}
		if _, err := tw.Write(data); err != nil {
			return "", err
		}
	}
	if err := tw.Close(); err != nil {
		return "", err
	}
	if _, err := s.build(ctx, &buf, tag, map[string]string{labelPrefix + "role": "copier"}, nil); err != nil {
		return "", fmt.Errorf("building the copy helper image: %w", err)
	}
	return tag, nil
}

/* ── Building a side image ──────────────────────────────── */

type SideImageOptions struct {
	ComparisonID string
	Side         string
	// Runtime is the base image for the project, e.g. "node:22-bookworm-slim". Debian-based for now.
	Runtime string
	// Setup runs once inside the image, e.g. "npm ci". Empty skips it.
	Setup string
	// CLIInstall installs the agent CLI, e.g. "npm install -g opencode-ai@1.18.34". It runs before
	// the project is copied, so its layer is cached across projects.
	CLIInstall string
	// HomeFiles are written under /root after the baseline commit (CLI configuration), keyed by
	// path relative to the home folder.
	HomeFiles map[string]string
}

type BuildResult struct {
	Image string
	// Log is the builder output, useful for the Logs tab.
	Log  string
	Took time.Duration
}

// BuildSideImage builds the image a side runs in: runtime, the copied project and a Git
// baseline commit so the final diff shows only what the agent changed.
func (s *Service) BuildSideImage(ctx context.Context, o SideImageOptions) (BuildResult, error) {
	start := time.Now()
	projectDir := filepath.Join(s.opts.StagingDir, o.ComparisonID, "project")
	if _, err := os.Stat(projectDir); err != nil {
		return BuildResult{}, fmt.Errorf("the project copy for %s is missing: %w", o.ComparisonID, err)
	}

	dockerfile := sideDockerfile(o)
	var buf bytes.Buffer
	if err := writeContext(&buf, projectDir, dockerfile, o.HomeFiles); err != nil {
		return BuildResult{}, fmt.Errorf("packing the build context: %w", err)
	}

	tag := fmt.Sprintf("ai-compare/side:%s-%s", strings.ToLower(o.ComparisonID), strings.ToLower(o.Side))
	labels := map[string]string{labelPrefix + "comparison": o.ComparisonID, labelPrefix + "side": o.Side, labelPrefix + "role": "side"}
	log, err := s.build(ctx, &buf, tag, labels, nil)
	if err != nil {
		return BuildResult{Log: log}, err
	}
	return BuildResult{Image: tag, Log: log, Took: time.Since(start)}, nil
}

func sideDockerfile(o SideImageOptions) string {
	runtime := o.Runtime
	if runtime == "" {
		runtime = "node:22-bookworm-slim"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "FROM %s\n", runtime)
	b.WriteString("RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*\n")
	if strings.TrimSpace(o.CLIInstall) != "" {
		fmt.Fprintf(&b, "RUN %s\n", o.CLIInstall)
	}
	b.WriteString("WORKDIR /workspace\n")
	b.WriteString("COPY project/ /workspace/\n")
	if strings.TrimSpace(o.Setup) != "" {
		fmt.Fprintf(&b, "RUN %s\n", o.Setup)
	}
	// The baseline commit holds the project exactly as copied; autocrlf=false keeps Windows line endings as they are.
	b.WriteString("RUN git init -q -b baseline && git config core.autocrlf false && git config user.name ai-compare && " +
		"git config user.email ai-compare@localhost && git add -A && git commit -q --allow-empty -m baseline\n")
	if len(o.HomeFiles) > 0 {
		b.WriteString("COPY home/ /root/\n")
	}
	return b.String()
}

// writeContext tars dir under "project/", the home files under "home/" and the Dockerfile.
func writeContext(w io.Writer, dir, dockerfile string, home map[string]string) error {
	tw := tar.NewWriter(w)
	files := map[string]string{"Dockerfile": dockerfile}
	for p, content := range home {
		files["home/"+strings.TrimPrefix(p, "/")] = content
	}
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), ModTime: time.Now()}); err != nil {
			return err
		}
		if _, err := io.WriteString(tw, content); err != nil {
			return err
		}
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == "." {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&fs.ModeSymlink != 0 {
			if link, err = os.Readlink(path); err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		hdr.Name = "project/" + filepath.ToSlash(rel)
		if d.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, f)
			f.Close()
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return tw.Close()
}

// build runs a classic (non-BuildKit) build from a tar context and returns the builder output.
func (s *Service) build(ctx context.Context, buildContext io.Reader, tag string, labels map[string]string, args map[string]*string) (string, error) {
	res, err := s.cli.ImageBuild(ctx, buildContext, client.ImageBuildOptions{
		Tags:        []string{tag},
		Labels:      labels,
		BuildArgs:   args,
		Remove:      true,
		ForceRemove: true,
		Version:     build.BuilderV1,
	})
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var log strings.Builder
	dec := json.NewDecoder(res.Body)
	for {
		var msg struct {
			Stream      string `json:"stream"`
			Status      string `json:"status"`
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := dec.Decode(&msg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return log.String(), fmt.Errorf("reading build output: %w", err)
		}
		log.WriteString(msg.Stream)
		if msg.Status != "" {
			log.WriteString(msg.Status + "\n")
		}
		if msg.Error != "" {
			return log.String(), fmt.Errorf("image build failed: %s", strings.TrimSpace(msg.Error))
		}
	}
	return log.String(), nil
}
