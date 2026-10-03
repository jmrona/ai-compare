package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"ai-compare/backend/internal/catalog"
	"ai-compare/backend/internal/proxy"
)

// caller makes model calls for one stage of a report through the inference proxy, with a session
// of its own so their cost is measured.
type caller struct {
	s       *Service
	model   string
	effort  string
	token   string
	session *proxy.Session
}

func (s *Service) newCaller(id, model string) (*caller, error) {
	var price *catalog.Price
	var long *catalog.LongContext
	effort := ""
	if cat, err := s.opts.Catalog.Get(context.Background()); err == nil {
		for _, m := range cat.Models {
			if m.Provider == "openai" && m.ID == model {
				price, long = m.Price, m.LongContext
				// Low effort is enough for reviewing and summarising, and keeps the report cheap.
				if slices.Contains(m.Efforts, "low") {
					effort = "low"
				}
			}
		}
	}
	token, session, err := s.opts.Proxy.NewSession("report-"+id, "openai", model, price, long, proxy.Limits{})
	if err != nil {
		return nil, err
	}
	return &caller{s: s, model: model, effort: effort, token: token, session: session}, nil
}

func (c *caller) close() { c.s.opts.Proxy.EndSession(c.token) }

func (c *caller) cost() *float64 { return c.session.Snapshot().CostUSD }

// ask sends one request to the OpenAI Responses API with a strict JSON schema and decodes the
// answer into out.
func (c *caller) ask(ctx context.Context, system, user, name string, schema map[string]any, out any) error {
	body := map[string]any{
		"model": c.model,
		"input": []map[string]any{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"text": map[string]any{"format": map[string]any{"type": "json_schema", "name": name, "schema": schema, "strict": true}},
	}
	if c.effort != "" {
		body["reasoning"] = map[string]any{"effort": c.effort}
	}
	data, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.s.opts.ProxyURL+"/openai/v1/responses", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	var parsed struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Refusal string `json:"refusal"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("%s returned %s: %s", c.model, res.Status, clip(string(raw), 300))
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return fmt.Errorf("%s: %s", c.model, parsed.Error.Message)
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", c.model, res.Status)
	}
	for _, o := range parsed.Output {
		if o.Type != "message" {
			continue
		}
		for _, part := range o.Content {
			if part.Refusal != "" {
				return fmt.Errorf("%s refused: %s", c.model, part.Refusal)
			}
			if part.Type == "output_text" {
				if err := json.Unmarshal([]byte(part.Text), out); err != nil {
					return fmt.Errorf("%s answered with invalid JSON: %w", c.model, err)
				}
				return nil
			}
		}
	}
	return fmt.Errorf("%s returned no answer (status %q)", c.model, parsed.Status)
}
