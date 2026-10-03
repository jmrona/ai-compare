package comparison

import (
	"encoding/json"
	"fmt"
)

// OpencodeVersion is pinned so two comparisons on the same day run the same CLI.
const OpencodeVersion = "1.18.34"

// agent describes how a CLI is installed, configured and started inside a side container.
type agent struct {
	install   string
	homeFiles map[string]string
	command   []string
	env       []string
}

// opencodeAgent points opencode at the inference proxy. The side token is passed as the API
// key, so the real key never enters the container.
func opencodeAgent(cfg SideConfig, prompt, proxyBaseURL, token string) (agent, error) {
	if cfg.Provider != "openai" {
		return agent{}, fmt.Errorf("opencode with %s is not supported yet (phase 2)", cfg.Provider)
	}
	model := map[string]any{}
	if cfg.Effort != "" {
		model["options"] = map[string]any{"reasoningEffort": cfg.Effort}
	}
	config := map[string]any{
		"$schema":    "https://opencode.ai/config.json",
		"autoupdate": false,
		"share":      "disabled",
		"model":      "openai/" + cfg.Model,
		// opencode uses a second model for titles; the same one avoids calling a model the user did not pick.
		"small_model": "openai/" + cfg.Model,
		"provider": map[string]any{
			"openai": map[string]any{
				"options": map[string]any{"baseURL": proxyBaseURL},
				"models":  map[string]any{cfg.Model: model},
			},
		},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return agent{}, err
	}

	a := agent{
		install:   "npm install -g opencode-ai@" + OpencodeVersion + " && npm cache clean --force",
		homeFiles: map[string]string{".config/opencode/opencode.json": string(data)},
		env:       []string{"OPENAI_API_KEY=" + token},
	}
	if cfg.Mode == "autonomous" {
		// Runs to completion without asking; the container is the safety boundary.
		a.command = []string{"opencode", "run", "--auto", "-m", "openai/" + cfg.Model, prompt}
	} else {
		// The TUI opens with the prompt already sent; the user answers in the browser terminal.
		a.command = []string{"opencode", "--prompt", prompt}
	}
	return a, nil
}
