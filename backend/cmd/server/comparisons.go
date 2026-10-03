package main

import (
	"log/slog"
	"net/http"

	"ai-compare/backend/internal/comparison"
)

// Plain HTTP routes for what Connect does not carry: the terminal WebSocket and file downloads.
// Everything else about comparisons is ComparisonService (internal/rpc).
func registerComparisonFiles(mux *http.ServeMux, svc *comparison.Service) {
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
	mux.HandleFunc("GET /api/comparisons/{id}/sides/{side}/recording", func(w http.ResponseWriter, r *http.Request) {
		path, err := svc.Recording(r.PathValue("id"), r.PathValue("side"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.Header().Set("Content-Type", "application/x-asciicast")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, path)
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
