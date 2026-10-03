package main

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"ai-compare/backend/internal/comparison"
)

// Plain JSON routes for the comparison flow, following frontend/src/api/http.ts. They move to
// Connect services (internal/rpc) one at a time; project inspection already has.
func registerComparisons(mux *http.ServeMux, svc *comparison.Service) {
	mux.HandleFunc("POST /api/comparisons", func(w http.ResponseWriter, r *http.Request) {
		var in comparison.NewComparison
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		id, err := svc.Start(r.Context(), in)
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
