package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"ai-compare/backend/internal/comparison"
	"ai-compare/backend/internal/workspace"
)

// Plain JSON routes for the comparison flow, following frontend/src/api/http.ts. They move to
// Connect services when the protobuf contract is written.
func registerComparisons(mux *http.ServeMux, svc *comparison.Service, ws *workspace.Service) {
	mux.HandleFunc("POST /api/projects/inspect", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Path) == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("send the project path as JSON: {\"path\": \"...\"}"))
			return
		}
		ins, err := ws.InspectProject(r.Context(), strings.TrimSpace(in.Path))
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, workspace.ErrPathNotFound) || errors.Is(err, workspace.ErrPathNotShared) || strings.Contains(err.Error(), "must be absolute") {
				status = http.StatusUnprocessableEntity
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, inspectionView(strings.TrimSpace(in.Path), ins))
	})

	mux.HandleFunc("POST /api/comparisons", func(w http.ResponseWriter, r *http.Request) {
		var in comparison.NewComparison
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		id, err := svc.Start(in)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	})
	mux.HandleFunc("GET /api/comparisons", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, svc.List())
	})
	mux.HandleFunc("GET /api/comparisons/active", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, svc.Active())
	})
	mux.HandleFunc("GET /api/comparisons/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := svc.Get(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})
	for action, fn := range map[string]func(id, side string) error{"finish": svc.Finish, "cancel": svc.Cancel} {
		mux.HandleFunc("POST /api/comparisons/{id}/sides/{side}/"+action, func(w http.ResponseWriter, r *http.Request) {
			if err := fn(r.PathValue("id"), r.PathValue("side")); err != nil {
				writeError(w, http.StatusNotFound, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
	mux.HandleFunc("GET /api/comparisons/{id}/sides/{side}/logs", func(w http.ResponseWriter, r *http.Request) {
		logs, err := svc.Logs(r.PathValue("id"), r.PathValue("side"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, logs)
	})
	mux.HandleFunc("GET /api/comparisons/{id}/sides/{side}/download", func(w http.ResponseWriter, r *http.Request) {
		d, err := svc.Workspace(r.Context(), r.PathValue("id"), r.PathValue("side"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		defer d.Tar.Close()
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+d.Filename+`"`)
		if err := tarToZip(w, d.Tar); err != nil {
			// Headers are already sent; the browser gets a truncated file.
			slog.Warn("download failed", "error", err)
		}
	})
	mux.HandleFunc("GET /api/comparisons/{id}/sides/{side}/terminal", func(w http.ResponseWriter, r *http.Request) {
		hub, err := svc.Hub(r.PathValue("id"), r.PathValue("side"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		hub.ServeHTTP(w, r)
	})
}

var harnessDirs = map[string]bool{".claude": true, ".agents": true, ".codex": true, ".opencode": true, ".cursor": true}

type harnessFile struct {
	Path   string   `json:"path"`
	ReadBy []string `json:"readBy"`
}

// Which CLI reads each harness file at the project root.
var harnessReaders = map[string][]string{
	"AGENTS.md":       {"opencode", "codex"},
	"CLAUDE.md":       {"claude"},
	"CLAUDE.local.md": {"claude"},
	"GEMINI.md":       {},
	".claude":         {"claude"},
	".agents":         {"codex"},
	".codex":          {"codex"},
	".opencode":       {"opencode"},
	"opencode.json":   {"opencode"},
	"opencode.jsonc":  {"opencode"},
	".mcp.json":       {"claude"},
	".cursor":         {},
	".cursorrules":    {},
}

func inspectionView(path string, ins workspace.Inspection) map[string]any {
	harness := []harnessFile{}
	for _, h := range ins.Harness {
		name := h
		if harnessDirs[h] {
			name += "/"
		}
		harness = append(harness, harnessFile{Path: name, ReadBy: harnessReaders[h]})
	}
	return map[string]any{
		"path":         path,
		"name":         filepath.Base(strings.ReplaceAll(path, `\`, "/")),
		"isGit":        ins.Git,
		"fileCount":    ins.Files,
		"sizeBytes":    ins.Bytes,
		"harnessFiles": harness,
		"excluded":     ins.EnvFiles,
		"profile":      detectProfile(ins.Markers),
	}
}

// detectProfile proposes the runtime and commands from the files at the project root.
// The runtime must have Node.js for now, because opencode is installed with npm.
func detectProfile(markers []string) comparison.Profile {
	p := comparison.Profile{Runtime: "node:22-bookworm-slim"}
	has := func(m string) bool { return slices.Contains(markers, m) }
	switch {
	case has("pnpm-lock.yaml"):
		p.Setup = "corepack enable && pnpm install --frozen-lockfile"
		p.Test = "pnpm test"
	case has("yarn.lock"):
		p.Setup = "corepack enable && yarn install --frozen-lockfile"
		p.Test = "yarn test"
	case has("package-lock.json"):
		p.Setup = "npm ci"
		p.Test = "npm test"
	case has("package.json"):
		p.Setup = "npm install"
		p.Test = "npm test"
	}
	return p
}
