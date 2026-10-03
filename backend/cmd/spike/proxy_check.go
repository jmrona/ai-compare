package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	v1 "ai-compare/backend/internal/gen/aicompare/v1"
	"ai-compare/backend/internal/gen/aicompare/v1/aicomparev1connect"
)

// checkScript runs inside a container on the agent network. It prints one JSON line per check.
const checkScript = `
const out = (check, v) => console.log(JSON.stringify({ check, ...v }));
async function attempt(url, opts = {}, ms = 5000) {
  try {
    const r = await fetch(url, { ...opts, signal: AbortSignal.timeout(ms) });
    return { status: r.status, body: (await r.text()).replace(/\s+/g, ' ').slice(0, 160) };
  } catch (e) {
    return { error: String(e.cause?.code || e.cause?.message || e.message) };
  }
}
(async () => {
  out('proxy', await attempt('http://api:4701/healthz'));
  out('ui-api', await attempt('http://api:4700/api/health'));
  out('postgres', await attempt('http://postgres:5432/'));
  out('internet', await attempt('https://models.dev/api.json', { method: 'HEAD' }));
  if (process.env.REAL !== '1') return;
  const headers = { Authorization: 'Bearer ' + process.env.TOKEN, 'Content-Type': 'application/json' };
  const model = process.env.MODEL;
  out('chat-completions', await attempt(process.env.BASE + '/chat/completions', { method: 'POST', headers,
    body: JSON.stringify({ model, messages: [{ role: 'user', content: 'Reply with the single word: ok' }] }) }, 60000));
  try {
    const r = await fetch(process.env.BASE + '/responses', { method: 'POST', headers,
      body: JSON.stringify({ model, input: 'Count from 1 to 5, one number per line.', stream: true }), signal: AbortSignal.timeout(60000) });
    let chunks = 0, first = 0; const start = Date.now();
    for await (const _ of r.body) { if (!chunks) first = Date.now() - start; chunks++; }
    out('responses-stream', { status: r.status, chunks, firstChunkMs: first, totalMs: Date.now() - start });
  } catch (e) { out('responses-stream', { error: String(e.message) }); }
})();
`

// proxyCheck covers spike points 3 and 6: a container on the agent network reaches the proxy
// and nothing else of the stack, and, with an OpenAI key, real requests (plain and streamed)
// go through the proxy, which records their usage and cost.
func proxyCheck(ctx context.Context, model string) error {
	apiURL := fmt.Sprintf("http://127.0.0.1:%s", env("APP_PORT", "4700"))
	cli, err := client.New(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	var health struct {
		Providers map[string]bool `json:"providers"`
	}
	if err := getJSON(apiURL+"/api/health", &health); err != nil {
		return err
	}
	real := health.Providers["openai"]
	if model == "" {
		if model, err = cheapestOpenAIModel(apiURL); err != nil {
			return err
		}
	}

	var session struct {
		ID       string `json:"id"`
		Token    string `json:"token"`
		BaseURL  string `json:"baseUrl"`
		PriceSet bool   `json:"priceSet"`
	}
	body, _ := json.Marshal(map[string]any{"provider": "openai", "model": model})
	res, err := http.Post(apiURL+"/api/spike/proxy/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	if err := decode(res, &session); err != nil {
		return err
	}
	fmt.Printf("session %s · model %s · price snapshot: %v\n", session.ID, model, session.PriceSet)
	if !real {
		fmt.Println("OPENAI_API_KEY is not set: only the network checks run (add it to .env for point 6 with real requests).")
	}

	image := "node:22-bookworm-slim"
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:  image,
			Cmd:    []string{"node", "-e", checkScript},
			Env:    []string{"TOKEN=" + session.Token, "BASE=" + session.BaseURL, "MODEL=" + model, "REAL=" + map[bool]string{true: "1", false: "0"}[real]},
			Labels: map[string]string{"ai-compare.role": "spike-proxy-check"},
		},
		HostConfig:       &container.HostConfig{NetworkMode: container.NetworkMode(env("AGENT_NETWORK", "ai-compare-agents"))},
		NetworkingConfig: &network.NetworkingConfig{},
	})
	if err != nil {
		return fmt.Errorf("creating the check container (is %s pulled? run the terminal spike once): %w", image, err)
	}
	defer cli.ContainerRemove(context.WithoutCancel(ctx), created.ID, client.ContainerRemoveOptions{Force: true})
	if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return err
	}
	wait := cli.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case <-wait.Result:
	case err := <-wait.Error:
		return err
	}
	logs, err := cli.ContainerLogs(ctx, created.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return err
	}
	var out, errOut bytes.Buffer
	stdcopy.StdCopy(&out, &errOut, logs)
	logs.Close()

	fmt.Println("\nfrom a container on the agent network:")
	expect := map[string]string{
		"proxy":    "reachable (expected)",
		"ui-api":   "must be refused with 403",
		"postgres": "must not resolve",
		"internet": "reachable (agents may install packages)",
	}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r map[string]any
		if json.Unmarshal([]byte(line), &r) != nil {
			continue
		}
		check := r["check"].(string)
		delete(r, "check")
		b, _ := json.Marshal(r)
		fmt.Printf("  %-17s %s  %s\n", check, b, expect[check])
	}
	if errOut.Len() > 0 {
		fmt.Fprintln(os.Stderr, errOut.String())
	}

	if real {
		time.Sleep(300 * time.Millisecond) // let the proxy finish recording the stream
		var snap struct {
			Usage    map[string]any   `json:"usage"`
			CostUSD  *float64         `json:"costUsd"`
			Requests []map[string]any `json:"requests"`
		}
		if err := getJSON(apiURL+"/api/spike/proxy/sessions/"+session.ID, &snap); err != nil {
			return err
		}
		fmt.Println("\nrecorded by the proxy:")
		for _, r := range snap.Requests {
			b, _ := json.Marshal(map[string]any{"path": r["path"], "status": r["status"], "streamed": r["streamed"], "usage": r["usage"], "costUsd": r["costUsd"], "seconds": r["durationSec"], "error": r["error"]})
			fmt.Printf("  %s\n", b)
		}
		u, _ := json.Marshal(snap.Usage)
		cost := "n/a"
		if snap.CostUSD != nil {
			cost = fmt.Sprintf("$%.6f", *snap.CostUSD)
		}
		fmt.Printf("  total: %s · cost %s\n", u, cost)
	}
	return nil
}

func cheapestOpenAIModel(apiURL string) (string, error) {
	// Through the generated Connect client, as a check of the contract.
	models := aicomparev1connect.NewCatalogServiceClient(connect.NewClient(connecthttp.NewTransport(http.DefaultClient, apiURL+"/api/rpc")))
	res, err := models.GetCatalog(context.Background(), &v1.GetCatalogRequest{})
	if err != nil {
		return "", fmt.Errorf("reading the catalogue: %w", err)
	}
	best, bestPrice := "", 0.0
	for _, m := range res.GetCatalog().GetModels() {
		if m.GetProvider() == "openai" && !m.GetDeprecated() && m.GetToolCall() && m.GetTextOutput() && m.GetPrice() != nil && (best == "" || m.GetPrice().GetInput() < bestPrice) {
			best, bestPrice = m.GetId(), m.GetPrice().GetInput()
		}
	}
	if best == "" {
		return "", fmt.Errorf("no OpenAI model with a price in the catalogue")
	}
	return best, nil
}

func getJSON(url string, v any) error {
	res, err := http.Get(url)
	if err != nil {
		return err
	}
	return decode(res, v)
}

func decode(res *http.Response, v any) error {
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s returned %d: %s", res.Request.URL.Path, res.StatusCode, b)
	}
	return json.Unmarshal(b, v)
}
