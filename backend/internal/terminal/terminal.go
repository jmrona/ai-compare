// Package terminal bridges a container's TTY and a browser terminal (xterm.js) over WebSocket.
//
// Protocol on the WebSocket:
//   - binary messages from the server: raw TTY output;
//   - binary messages from the browser: keystrokes, written to the TTY as they are (so Ctrl+C
//     arrives as \x03 and the process in the container gets SIGINT from the TTY);
//   - text messages from the browser: JSON control messages, for now {"type":"resize","cols":N,"rows":N}.
package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type control struct {
	Type string `json:"type"`
	Cols uint   `json:"cols"`
	Rows uint   `json:"rows"`
}

// Attach connects to a container created with Tty and OpenStdin. Attach before starting the
// container so its first output (the prompt) is not lost.
func Attach(ctx context.Context, cli *client.Client, containerID string) (client.ContainerAttachResult, error) {
	attached, err := cli.ContainerAttach(ctx, containerID, client.ContainerAttachOptions{Stream: true, Stdin: true, Stdout: true, Stderr: true})
	if err != nil {
		return attached, fmt.Errorf("attaching to the container: %w", err)
	}
	return attached, nil
}

// Bridge copies data between the WebSocket and an attached container until either side ends.
func Bridge(ctx context.Context, cli *client.Client, conn *websocket.Conn, containerID string, attached client.ContainerAttachResult, log *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer attached.Close()

	errs := make(chan error, 2)

	// Container → browser. With a TTY the stream is raw, not multiplexed.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := attached.Reader.Read(buf)
			if n > 0 {
				if werr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					errs <- werr
					return
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				errs <- err
				return
			}
		}
	}()

	// Browser → container.
	go func() {
		for {
			typ, data, err := conn.Read(ctx)
			if err != nil {
				errs <- err
				return
			}
			if typ == websocket.MessageText {
				var c control
				if json.Unmarshal(data, &c) == nil && c.Type == "resize" && c.Cols > 0 && c.Rows > 0 {
					if _, err := cli.ContainerResize(ctx, containerID, client.ContainerResizeOptions{Width: c.Cols, Height: c.Rows}); err != nil {
						log.Warn("terminal resize failed", "container", containerID, "error", err)
					}
				}
				continue
			}
			if _, err := attached.Conn.Write(data); err != nil {
				errs <- err
				return
			}
		}
	}()

	err := <-errs
	if websocket.CloseStatus(err) == websocket.StatusNormalClosure || websocket.CloseStatus(err) == websocket.StatusGoingAway {
		return nil
	}
	return err
}

// SpikeHandler serves phase 0 spike point 4: each WebSocket connection starts a throwaway
// container running bash with a TTY and bridges it to the browser. The container is removed
// when the connection closes.
//
// GET /api/spike/terminal?image=<image>   (default node:22-bookworm-slim)
func SpikeHandler(cli *client.Client, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Same-origin only (the default): another site open in the browser cannot connect.
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()

		image := r.URL.Query().Get("image")
		if image == "" {
			image = "node:22-bookworm-slim"
		}
		fail := func(msg string, err error) {
			log.Warn(msg, "error", err)
			conn.Write(ctx, websocket.MessageBinary, []byte("\r\n\x1b[91m[ai-compare] "+msg+": "+err.Error()+"\x1b[0m\r\n"))
			conn.Close(websocket.StatusInternalError, msg)
		}

		if err := ensureImage(ctx, cli, image, conn); err != nil {
			fail("could not get the image", err)
			return
		}

		created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
			Config: &container.Config{
				Image:        image,
				Cmd:          []string{"bash", "-l"},
				Tty:          true,
				OpenStdin:    true,
				AttachStdin:  true,
				AttachStdout: true,
				AttachStderr: true,
				Env:          []string{"TERM=xterm-256color", "LANG=C.UTF-8"},
				Labels:       map[string]string{"ai-compare.role": "spike-terminal"},
			},
		})
		if err != nil {
			fail("could not create the container", err)
			return
		}
		id := created.ID
		defer func() {
			rm, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cli.ContainerRemove(rm, id, client.ContainerRemoveOptions{Force: true})
			log.Info("spike terminal closed", "container", id[:12])
		}()

		attached, err := Attach(ctx, cli, id)
		if err != nil {
			fail("could not attach to the container", err)
			return
		}
		if _, err := cli.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
			attached.Close()
			fail("could not start the container", err)
			return
		}
		log.Info("spike terminal started", "container", id[:12], "image", image)

		if err := Bridge(ctx, cli, conn, id, attached, log); err != nil && ctx.Err() == nil {
			log.Warn("spike terminal ended with an error", "error", err)
		}
		conn.Close(websocket.StatusNormalClosure, "the shell exited")
	})
}

func ensureImage(ctx context.Context, cli *client.Client, image string, conn *websocket.Conn) error {
	if _, err := cli.ImageInspect(ctx, image); err == nil {
		return nil
	}
	conn.Write(ctx, websocket.MessageBinary, []byte("\x1b[90m[ai-compare] pulling "+image+"…\x1b[0m\r\n"))
	pull, err := cli.ImagePull(ctx, image, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer pull.Close()
	return pull.Wait(ctx)
}
