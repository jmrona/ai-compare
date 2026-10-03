package proxy

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ai-compare/backend/internal/catalog"
)

func f(v float64) *float64 { return &v }

// fakeOpenAI answers like OpenAI and records the API key it was given.
func fakeOpenAI(t *testing.T, gotKey *string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotKey = r.Header.Get("Authorization")
		fakeProvider(w, r)
	}))
}

// fakeProvider serves the three APIs the proxy reads usage from.
func fakeProvider(w http.ResponseWriter, r *http.Request) {
	{
		body, _ := io.ReadAll(r.Body)
		stream := strings.Contains(string(body), `"stream":true`)
		switch {
		case r.URL.Path == "/v1/chat/completions" && !stream:
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"c1","choices":[{"message":{"content":"hi"}}],
				"usage":{"prompt_tokens":1200,"completion_tokens":300,"prompt_tokens_details":{"cached_tokens":1000}}}`)
		case r.URL.Path == "/v1/responses" && stream:
			w.Header().Set("Content-Type", "text/event-stream")
			fl := w.(http.Flusher)
			for _, ev := range []string{
				`{"type":"response.created","response":{"id":"r1"}}`,
				`{"type":"response.output_text.delta","delta":"Hel"}`,
				`{"type":"response.output_text.delta","delta":"lo"}`,
				`{"type":"response.completed","response":{"id":"r1","usage":{"input_tokens":500,"output_tokens":40,"input_tokens_details":{"cached_tokens":100}}}}`,
			} {
				fmt.Fprintf(w, "event: x\ndata: %s\n\n", ev)
				fl.Flush()
				time.Sleep(5 * time.Millisecond)
			}
		case r.URL.Path == "/v1/messages":
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":20,\"cache_read_input_tokens\":3000,\"cache_creation_input_tokens\":400,\"output_tokens\":1}}}\n\n")
			io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":250}}\n\n")
			io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}
}

func newProxy(upstream string) *Proxy {
	return New([]Provider{
		{Name: "openai", BaseURL: upstream, APIKey: "sk-real", AuthHeader: "Authorization"},
		{Name: "anthropic", BaseURL: upstream, APIKey: "ant-real", AuthHeader: "x-api-key"},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func post(t *testing.T, url, token, body string) (*http.Response, string) {
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestProxyNonStreamedChatCompletions(t *testing.T) {
	var key string
	up := fakeOpenAI(t, &key)
	defer up.Close()
	p := newProxy(up.URL)
	srv := httptest.NewServer(p)
	defer srv.Close()

	price := &catalog.Price{Input: 2, CacheRead: f(0.1), Output: 10}
	token, s, _ := p.NewSession("A", "openai", "gpt-x", price, nil, Limits{})
	res, body := post(t, srv.URL+"/openai/v1/chat/completions", token, `{"model":"gpt-x"}`)

	if res.StatusCode != 200 || !strings.Contains(body, `"content":"hi"`) {
		t.Fatalf("response %d %s", res.StatusCode, body)
	}
	if key != "Bearer sk-real" {
		t.Fatalf("upstream got key %q, want the real one", key)
	}
	snap := s.Snapshot()
	want := Usage{Input: 200, CacheRead: 1000, Output: 300, Reported: true}
	if snap.Usage != want {
		t.Fatalf("usage = %+v, want %+v", snap.Usage, want)
	}
	// 200*2 + 1000*0.1 + 300*10 = 3500 per million
	if snap.CostUSD == nil || !near(*snap.CostUSD, 0.0035) {
		t.Fatalf("cost = %v", snap.CostUSD)
	}
}

func TestProxyStreamsResponsesAPIUnchanged(t *testing.T) {
	var key string
	up := fakeOpenAI(t, &key)
	defer up.Close()
	p := newProxy(up.URL)
	srv := httptest.NewServer(p)
	defer srv.Close()

	token, s, _ := p.NewSession("B", "openai", "gpt-x", &catalog.Price{Input: 1, Output: 4}, nil, Limits{})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/openai/v1/responses", strings.NewReader(`{"stream":true}`))
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	// Events must arrive one by one, not all at the end.
	sc := bufio.NewScanner(res.Body)
	var deltas int
	for sc.Scan() {
		if strings.Contains(sc.Text(), "output_text.delta") {
			deltas++
		}
	}
	res.Body.Close()
	if deltas != 2 {
		t.Fatalf("got %d deltas through the proxy", deltas)
	}

	snap := s.Snapshot()
	if snap.Usage != (Usage{Input: 400, CacheRead: 100, Output: 40, Reported: true}) {
		t.Fatalf("usage = %+v", snap.Usage)
	}
	if len(snap.Requests) != 1 || !snap.Requests[0].Streamed {
		t.Fatalf("requests = %+v", snap.Requests)
	}
	// No cache-read price: cached input is billed as input. (400+100)*1 + 40*4 = 660 per million.
	if snap.CostUSD == nil || !near(*snap.CostUSD, 0.00066) {
		t.Fatalf("cost = %v", snap.CostUSD)
	}
}

func TestProxyAnthropicStreamAndHeader(t *testing.T) {
	var key string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("x-api-key")
		fakeProvider(w, r)
	}))
	defer up.Close()
	p := newProxy(up.URL)
	srv := httptest.NewServer(p)
	defer srv.Close()

	token, s, _ := p.NewSession("C", "anthropic", "claude-x", &catalog.Price{Input: 3, CacheRead: f(0.3), CacheWrite: f(3.75), Output: 15}, nil, Limits{})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/anthropic/v1/messages", strings.NewReader(`{"stream":true}`))
	req.Header.Set("x-api-key", token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()

	if key != "ant-real" {
		t.Fatalf("upstream got x-api-key %q", key)
	}
	if got := s.Snapshot().Usage; got != (Usage{Input: 20, CacheRead: 3000, CacheWrite: 400, Output: 250, Reported: true}) {
		t.Fatalf("usage = %+v", got)
	}
}

func TestProxyRejectsUnknownTokenAndEnforcesLimits(t *testing.T) {
	var key string
	up := fakeOpenAI(t, &key)
	defer up.Close()
	p := newProxy(up.URL)
	srv := httptest.NewServer(p)
	defer srv.Close()

	if res, _ := post(t, srv.URL+"/openai/v1/chat/completions", "sk-not-a-session", `{}`); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown token: status %d", res.StatusCode)
	}
	if key != "" {
		t.Fatal("an unknown token must not reach the provider")
	}

	token, s, _ := p.NewSession("D", "openai", "gpt-x", &catalog.Price{Input: 2, Output: 10}, nil, Limits{MaxTokens: 1000})
	if res, _ := post(t, srv.URL+"/anthropic/v1/messages", token, `{}`); res.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong provider: status %d", res.StatusCode)
	}
	post(t, srv.URL+"/openai/v1/chat/completions", token, `{}`) // 1500 tokens: crosses the limit
	res, body := post(t, srv.URL+"/openai/v1/chat/completions", token, `{}`)
	if res.StatusCode != http.StatusForbidden || !strings.Contains(body, "token limit") {
		t.Fatalf("over the limit: %d %s", res.StatusCode, body)
	}
	if s.Snapshot().LimitHit != "token limit" || len(s.Snapshot().Requests) != 1 {
		t.Fatalf("snapshot = %+v", s.Snapshot())
	}
}

func TestCostUsesLongContextTier(t *testing.T) {
	base := &catalog.Price{Input: 2, Output: 10}
	long := &catalog.LongContext{AboveTokens: 272000, Price: catalog.Price{Input: 4, Output: 15}}
	short := Cost(Usage{Input: 100_000, Output: 1000, Reported: true}, base, long)
	big := Cost(Usage{Input: 300_000, Output: 1000, Reported: true}, base, long)
	if !near(*short, 0.21) || !near(*big, 1.215) {
		t.Fatalf("short=%v big=%v", *short, *big)
	}
	if Cost(Usage{}, base, long) != nil {
		t.Fatal("unreported usage must have no cost")
	}
	if Cost(Usage{Input: 1, Reported: true}, nil, nil) != nil {
		t.Fatal("no price must mean no cost")
	}
}
