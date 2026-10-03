// Command spike exercises phase 0 spike points 1 and 2 from inside the api container:
//
//	docker compose exec api /app/spike prepare "C:\Users\me\projects\my-app"
//	docker compose exec api /app/spike prepare /Users/me/projects/my-app --setup "npm ci"
//
// It copies the project into the staging volume through a helper container (point 1)
// and builds the image for side A from that copy (point 2), printing timings.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"ai-compare/backend/internal/workspace"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "prepare" {
		fmt.Fprintln(os.Stderr, "usage: spike prepare <absolute project path> [--runtime image] [--setup command]")
		os.Exit(2)
	}
	path := os.Args[2]
	flags := flag.NewFlagSet("prepare", flag.ExitOnError)
	runtime := flags.String("runtime", "node:22-bookworm-slim", "base image for the side")
	setup := flags.String("setup", "", "command to run once in the image, e.g. \"npm ci\"")
	flags.Parse(os.Args[3:])

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cli, err := workspace.NewDockerClient()
	if err != nil {
		fail("connecting to Docker", err)
	}
	ws := workspace.New(cli, workspace.Options{
		StagingVolume: env("STAGING_VOLUME", "ai-compare_staging"),
		StagingDir:    env("STAGING_DIR", "/data/staging"),
		Log:           log,
	})

	id := fmt.Sprintf("spike-%d", time.Now().Unix())
	fmt.Printf("comparison id: %s\n", id)

	copied, err := ws.CopyProject(ctx, path, id)
	if err != nil {
		fail("point 1, copying the project", err)
	}
	fmt.Printf("point 1 ok: copied %d files (%d KB, %s mode, %d .env files skipped) in %s\n",
		copied.Files, copied.Kilobytes, copied.Mode, copied.EnvFilesSkipped, copied.Took.Round(time.Millisecond))

	built, err := ws.BuildSideImage(ctx, workspace.SideImageOptions{ComparisonID: id, Side: "A", Runtime: *runtime, Setup: *setup})
	if err != nil {
		fmt.Fprintln(os.Stderr, tail(built.Log, 20))
		fail("point 2, building the side image", err)
	}
	fmt.Printf("point 2 ok: built %s in %s\n", built.Image, built.Took.Round(time.Millisecond))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "%s failed: %v\n", step, err)
	os.Exit(1)
}
