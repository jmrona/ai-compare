package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ai-compare/backend/internal/catalog"
	"ai-compare/backend/internal/config"
	"ai-compare/backend/internal/proxy"
)

// registerProxySpike adds phase 0 endpoints to create proxy sessions by hand and inspect them.
// The orchestrator will create sessions itself once comparisons run for real.
//
//	POST /api/spike/proxy/sessions      {"provider":"openai","model":"gpt-6-luna","maxTokens":0,"maxCostUsd":0}
//	GET  /api/spike/proxy/sessions/{id}
func registerProxySpike(mux *http.ServeMux, p *proxy.Proxy, models *catalog.Service, cfg config.Config) {
	mux.HandleFunc("POST /api/spike/proxy/sessions", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Provider   string  `json:"provider"`
			Model      string  `json:"model"`
			MaxTokens  int64   `json:"maxTokens"`
			MaxCostUSD float64 `json:"maxCostUsd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Provider == "" || in.Model == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("send provider and model as JSON"))
			return
		}
		// Snapshot the price now, as a real comparison will when it starts.
		var price *catalog.Price
		var long *catalog.LongContext
		if c, err := models.Get(r.Context()); err == nil {
			for _, m := range c.Models {
				if m.Provider == in.Provider && m.ID == in.Model {
					price, long = m.Price, m.LongContext
				}
			}
		}
		id := fmt.Sprintf("spike-%s-%s-%d", in.Provider, in.Model, time.Now().UnixMilli())
		token, _, err := p.NewSession(id, in.Provider, in.Model, price, long, proxy.Limits{MaxTokens: in.MaxTokens, MaxCostUSD: in.MaxCostUSD})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":       id,
			"token":    token,
			"baseUrl":  fmt.Sprintf("http://api:%d/%s/v1", cfg.ProxyPort, in.Provider),
			"price":    price,
			"priceSet": price != nil,
		})
	})
	mux.HandleFunc("GET /api/spike/proxy/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		s := p.Session(r.PathValue("id"))
		if s == nil {
			writeError(w, http.StatusNotFound, fmt.Errorf("no session %q", r.PathValue("id")))
			return
		}
		writeJSON(w, http.StatusOK, s.Snapshot())
	})
}
