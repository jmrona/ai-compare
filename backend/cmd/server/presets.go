package main

import (
	"bytes"
	"mime"
	"net/http"

	"ai-compare/backend/internal/presets"
)

func registerPresetFiles(mux *http.ServeMux, store *presets.Store) {
	mux.HandleFunc("GET /api/presets/{slug}/download", func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		name, err := store.WriteZip(r.PathValue("slug"), &buf)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		w.Write(buf.Bytes())
	})
}
