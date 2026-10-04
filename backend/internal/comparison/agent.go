package comparison

import (
	"encoding/json"
	"fmt"
	"sort"
)

// OpencodeVersion is pinned so two comparisons on the same day run the same CLI.
const OpencodeVersion = "1.18.34"

// agent describes how a CLI is installed, configured and started inside a side container.
type agent struct {
	install     string
	systemFiles map[string]string
	command     []string
	env         []string
}

const managedConfig = "/etc/opencode/opencode.json"

// opencodeProviders are the providers opencode runs in ai-compare, with the environment variable
// its built-in provider reads the API key from.
var opencodeProviders = map[string]string{
	"openai":    "OPENAI_API_KEY",
	"anthropic": "ANTHROPIC_API_KEY",
}

// AutonomousNote is appended to the prompt of an autonomous side: nobody is there to answer, so
// a question would end the run without changes.
const AutonomousNote = "\n\n---\nYou are running unattended: nobody will answer questions or approve steps. " +
	"Do not ask questions or wait for confirmation. When something is ambiguous, choose the option you judge best, " +
	"state that assumption in your final message, and carry the task through to the end."

// opencodeAgent points opencode at the inference proxy. The side token is passed as the API
// key, so the real key never enters the container.
func opencodeAgent(cfg SideConfig, prompt, proxyRoot, token string) (agent, error) {
	if _, ok := opencodeProviders[cfg.Provider]; !ok {
		return agent{}, fmt.Errorf("opencode with %s is not supported", cfg.Provider)
	}
	ref := cfg.Provider + "/" + cfg.Model
	model := map[string]any{}
	if cfg.Effort != "" && cfg.Provider == "openai" {
		model["options"] = map[string]any{"reasoningEffort": cfg.Effort}
	}
	build := map[string]any{"model": ref}
	if cfg.Effort != "" {
		// opencode's model variants are the reasoning efforts models.dev lists, for every provider.
		build["variant"] = cfg.Effort
	}
	config := map[string]any{
		"$schema":    "https://opencode.ai/config.json",
		"autoupdate": false,
		"share":      "disabled",
		"model":      ref,
		// opencode uses a second model for titles; the same one avoids calling a model the user did not pick.
		"small_model": ref,
		"agent":       map[string]any{"build": build},
		"provider":    map[string]any{},
	}
	providers := config["provider"].(map[string]any)
	var env []string
	for name, keyVar := range opencodeProviders {
		p := map[string]any{"options": map[string]any{"baseURL": proxyRoot + "/" + name + "/v1"}}
		if name == cfg.Provider {
			p["models"] = map[string]any{cfg.Model: model}
		}
		providers[name] = p
		env = append(env, keyVar+"="+token)
	}
	sort.Strings(env)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return agent{}, err
	}

	a := agent{
		install:     "npm install -g opencode-ai@" + OpencodeVersion + " && npm cache clean --force",
		systemFiles: map[string]string{managedConfig: string(data)},
		env:         env,
	}
	if cfg.Mode == "autonomous" {
		// Runs to completion without asking; the container is the safety boundary.
		a.command = []string{"opencode", "run", "--auto", "-m", ref}
		if cfg.Effort != "" {
			a.command = append(a.command, "--variant", cfg.Effort)
		}
		a.command = append(a.command, prompt+AutonomousNote)
	} else {
		// The TUI opens with the prompt already sent; the user answers in the browser terminal.
		a.command = []string{"opencode", "--prompt", prompt}
	}
	return a, nil
}
