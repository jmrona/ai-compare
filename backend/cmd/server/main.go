// Command server runs the ai-compare API.
//
// It listens on two ports:
//   - APP_PORT (published on 127.0.0.1): the UI and its API;
//   - PROXY_PORT (internal only): the inference proxy used by agent containers.
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
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ai-compare/backend/internal/catalog"
	"ai-compare/backend/internal/comparison"
	"ai-compare/backend/internal/config"
	"ai-compare/backend/internal/db"
	"ai-compare/backend/internal/netguard"
	"ai-compare/backend/internal/presets"
	"ai-compare/backend/internal/proxy"
	"ai-compare/backend/internal/report"
	"ai-compare/backend/internal/rpc"
	"ai-compare/backend/internal/settings"
	"ai-compare/backend/internal/terminal"
	"ai-compare/backend/internal/workspace"
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

	inference := proxy.New([]proxy.Provider{
		{Name: "openai", BaseURL: "https://api.openai.com", APIKey: cfg.OpenAIKey, AuthHeader: "Authorization"},
		{Name: "anthropic", BaseURL: "https://api.anthropic.com", APIKey: cfg.AnthropicKey, AuthHeader: "x-api-key"},
		// Clients call /local/v1/..., so the base drops the trailing /v1 of the configured URL.
		{Name: "local", BaseURL: strings.TrimSuffix(strings.TrimSuffix(cfg.LocalBaseURL, "/"), "/v1")},
	}, log)

	// Without Postgres the app still runs, but comparisons are lost when it stops.
	var queries *db.Queries
	var pool *pgxpool.Pool
	if cfg.DatabaseURL == "" {
		log.Warn("DATABASE_URL is not set; comparisons are kept in memory only")
	} else if pool, err = db.Open(context.Background(), cfg.DatabaseURL, log); err != nil {
		log.Error("database unavailable; comparisons are kept in memory only", "error", err)
	} else {
		defer pool.Close()
		queries = db.New(pool)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		database := "not configured"
		if pool != nil {
			database = "ok"
			if err := pool.Ping(r.Context()); err != nil {
				database = err.Error()
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":   "ok",
			"database": database,
			"providers": map[string]bool{
				"openai":    cfg.OpenAIKey != "",
				"anthropic": cfg.AnthropicKey != "",
			},
		})
	})
	registerProxySpike(mux, inference, models, cfg)

	prefs := settings.New(context.Background(), queries, log)
	harnesses, err := presets.New(filepath.Join(cfg.DataDir, "harnesses"))
	if err != nil {
		log.Error("presets are unavailable", "error", err)
	}

	var guard *netguard.Guard
	var ws *workspace.Service
	var comparisons *comparison.Service
	var reports *report.Service
	if docker, err := workspace.NewDockerClient(); err != nil {
		log.Warn("docker is not reachable; comparisons and terminals are disabled", "error", err)
	} else {
		// Phase 0 spike: a throwaway bash container bridged to the browser terminal.
		mux.Handle("GET /api/spike/terminal", terminal.SpikeHandler(docker, log))

		ws = workspace.New(docker, workspace.Options{
			StagingVolume: cfg.StagingVolume, StagingDir: cfg.StagingDir,
			ArtifactsVolume: cfg.ArtifactsVolume, ArtifactsDir: cfg.ArtifactsDir, Log: log,
		})
		comparisons = comparison.New(comparison.Options{
			Docker: docker, Workspace: ws, Proxy: inference, Catalog: models, Settings: prefs, Presets: harnesses,
			AgentNetwork: cfg.AgentNetwork, ProxyPort: cfg.ProxyPort, DB: queries, Log: log,
		})
		reports = report.New(report.Options{
			Comparisons: comparisons, Proxy: inference, Catalog: models, Settings: prefs,
			ProxyURL: fmt.Sprintf("http://127.0.0.1:%d", cfg.ProxyPort), Log: log,
		})
		comparisons.SetReporter(reports)
		if err := comparisons.Load(context.Background()); err != nil {
			log.Error("could not load saved comparisons", "error", err)
		}
		reports.Recover(context.Background())
		go comparisons.RetentionLoop(context.Background())
		registerComparisonFiles(mux, comparisons)
		if guard, err = netguard.ForNetwork(context.Background(), docker, cfg.AgentNetwork); err != nil {
			log.Warn("agent network not found; its containers are not blocked from the API", "error", err)
		} else {
			log.Info("blocking the agent network from the API", "network", cfg.AgentNetwork, "subnets", guard.Subnets())
		}
	}

	// Connect services (proto/aicompare/v1): everything but terminals and file downloads.
	mux.Handle("/api/rpc/", http.StripPrefix("/api/rpc", rpc.Handler(rpc.Deps{
		Catalog: models, Settings: prefs, Workspace: ws, Comparisons: comparisons, Reports: reports, Presets: harnesses, HostHome: cfg.HostHome,
		Env: rpc.Env{OpenAIKey: cfg.OpenAIKey != "", AnthropicKey: cfg.AnthropicKey != "", LocalBaseURL: cfg.LocalBaseURL},
	})))

	// Unknown API routes get a JSON 404 instead of falling through to the frontend.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, fmt.Errorf("%s %s does not exist", r.Method, r.URL.Path))
	})
	if cfg.StaticDir != "" {
		mux.Handle("/", spaHandler(cfg.StaticDir))
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.AppPort),
		Handler:           guard.Block(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// No write timeout: model responses can stream for minutes.
	proxySrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.ProxyPort),
		Handler:           inference,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for name, s := range map[string]*http.Server{"api": srv, "proxy": proxySrv} {
		go func() {
			log.Info(name+" listening", "addr", s.Addr)
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error(name+" stopped", "error", err)
				stop()
			}
		}()
	}

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, s := range []*http.Server{srv, proxySrv} {
		if err := s.Shutdown(shutdownCtx); err != nil {
			log.Error("shutdown incomplete", "addr", s.Addr, "error", err)
		}
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
