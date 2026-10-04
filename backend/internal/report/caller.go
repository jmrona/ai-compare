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
	"ai-compare/backend/internal/settings"
)

type caller struct {
	s        *Service
	provider string
	model    string
	effort   string
	token    string
	session  *proxy.Session
}

func (s *Service) newCaller(id, ref, effort string) (*caller, error) {
	provider, model := settings.ModelRef(ref)
	var price *catalog.Price
	var long *catalog.LongContext
	chosen := ""
	if cat, err := s.opts.Catalog.Get(context.Background()); err == nil {
		for _, m := range cat.Models {
			if m.Provider == provider && m.ID == model {
				price, long = m.Price, m.LongContext
				if slices.Contains(m.Efforts, effort) {
					chosen = effort
				}
			}
		}
	}
	token, session, err := s.opts.Proxy.NewSession("report-"+id, provider, model, price, long, proxy.Limits{})
	if err != nil {
		return nil, err
	}
	return &caller{s: s, provider: provider, model: model, effort: chosen, token: token, session: session}, nil
}

func (c *caller) close() { c.s.opts.Proxy.EndSession(c.token) }

func (c *caller) cost() *float64 { return c.session.Snapshot().CostUSD }

func (c *caller) ask(ctx context.Context, system, user, name string, schema map[string]any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if c.provider == "anthropic" {
		return c.askAnthropic(ctx, system, user, name, schema, out)
	}
	return c.askOpenAI(ctx, system, user, name, schema, out)
}

func (c *caller) post(ctx context.Context, path string, body any, headers map[string]string) (*http.Response, []byte, error) {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.s.opts.ProxyURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	return res, raw, nil
}

func (c *caller) askOpenAI(ctx context.Context, system, user, name string, schema map[string]any, out any) error {
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
	res, raw, err := c.post(ctx, "/openai/v1/responses", body, map[string]string{"Authorization": "Bearer " + c.token})
	if err != nil {
		return err
	}
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

func (c *caller) askAnthropic(ctx context.Context, system, user, name string, schema map[string]any, out any) error {
	body := map[string]any{
		"model":       c.model,
		"max_tokens":  16000,
		"system":      system,
		"messages":    []map[string]any{{"role": "user", "content": user}},
		"tools":       []map[string]any{{"name": name, "description": "Return the answer.", "input_schema": schema}},
		"tool_choice": map[string]any{"type": "tool", "name": name},
	}
	res, raw, err := c.post(ctx, "/anthropic/v1/messages", body, map[string]string{"x-api-key": c.token, "anthropic-version": "2023-06-01"})
	if err != nil {
		return err
	}
	var parsed struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
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
	for _, part := range parsed.Content {
		if part.Type == "tool_use" && part.Name == name {
			if err := json.Unmarshal(part.Input, out); err != nil {
				return fmt.Errorf("%s answered with invalid JSON: %w", c.model, err)
			}
			return nil
		}
	}
	return fmt.Errorf("%s returned no answer (stop reason %q)", c.model, parsed.StopReason)
}
