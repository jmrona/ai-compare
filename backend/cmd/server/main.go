// Command server runs the ai-compare API.
//
// So far: a health check, the models.dev catalogue and, when STATIC_DIR is set, the built frontend.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ai-compare/backend/internal/catalog"
	"ai-compare/backend/internal/config"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("could not load configuration", "error", err)
		os.Exit(1)
	}
	if cfg.EnvFile != "" {
		log.Info("configuration loaded", "env_file", cfg.EnvFile)
	}

	models := catalog.New(catalog.Options{
		URL:       cfg.ModelsDevURL,
		Providers: cfg.CatalogProviders,
		CacheFile: filepath.Join(cfg.DataDir, "catalog.json"),
		Log:       log,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"providers": map[string]bool{
				"openai":    cfg.OpenAIKey != "",
				"anthropic": cfg.AnthropicKey != "",
			},
		})
	})
	mux.HandleFunc("GET /api/catalog", func(w http.ResponseWriter, r *http.Request) {
		c, err := models.Get(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	})
	mux.HandleFunc("POST /api/catalog/refresh", func(w http.ResponseWriter, r *http.Request) {
		c, err := models.Refresh(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	})
	// Unknown API routes get a JSON 404 instead of falling through to the frontend.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, fmt.Errorf("%s %s does not exist", r.Method, r.URL.Path))
	})
	if cfg.StaticDir != "" {
		mux.Handle("/", spaHandler(cfg.StaticDir))
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.AppPort),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("api listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server stopped", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown incomplete", "error", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// spaHandler serves files from dir and falls back to index.html so client-side
// routes like /history/0142 load the app.
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}
