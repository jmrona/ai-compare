# 13. Agent CLIs

Code: `backend/internal/comparison/agent.go`.

An **agent adapter** answers four questions for a CLI:

| Question | Field | Example for opencode |
|---|---|---|
| How is it installed in the image? | `install` | `npm install -g opencode-ai@1.18.34 && npm cache clean --force` |
| Which files configure it? | `homeFiles` (written under `/root`, after the baseline commit) | `.config/opencode/opencode.json` |
| Which environment does it need? | `env` | `OPENAI_API_KEY=<side token>` |
| What command starts it? | `command` | see below |

## opencode (phase 1)

**Pinned version:** `1.18.34` (`OpencodeVersion`). Two comparisons on different days must run the same CLI, and the version is shown with each side. `autoupdate: false` stops it from updating itself.

Generated `~/.config/opencode/opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "autoupdate": false,
  "share": "disabled",
  "model": "openai/gpt-5.4-mini",
  "small_model": "openai/gpt-5.4-mini",
  "provider": {
    "openai": {
      "options": { "baseURL": "http://api:4701/openai/v1" },
      "models": { "gpt-5.4-mini": { "options": { "reasoningEffort": "low" } } }
    }
  }
}
```

- **`baseURL`** points the built-in OpenAI provider at the proxy, so all traffic is measured and the real key is never needed.
- **`small_model`** is set to the same model. opencode uses a second, smaller model for things like session titles; left alone it would call a model the user did not choose (and, during the spike, one that was not covered by free usage).
- **`share: "disabled"`** keeps sessions from being shared online.
- **`reasoningEffort`** carries the effort chosen in the UI, only when the model has efforts.

Commands:

| Mode | Command | Behaviour |
|---|---|---|
| `autonomous` (default) | `opencode run --auto -m openai/<model> <prompt>` | Runs to completion without asking for permission; the container is the safety boundary. The side ends when the CLI exits |
| `interactive` | `opencode --prompt <prompt>` | The TUI opens with the prompt already sent; the user keeps the conversation going from the browser terminal and ends the side with Finish |

Both sides default to autonomous so they finish on their own. Interactive sides stay open until the user finishes them, which is intended (the user may want to ask follow-up questions) but means their agent time includes the user's.

## Harness files

Phase 1 copies the project's own harness as it is. The inspection tells the user which files were found and which CLI reads each:

| File | Read by |
|---|---|
| `AGENTS.md` | opencode, codex |
| `CLAUDE.md`, `CLAUDE.local.md`, `.claude/`, `.mcp.json` | claude |
| `.opencode/`, `opencode.json`, `opencode.jsonc` | opencode |
| `.agents/`, `.codex/` | codex |
| `GEMINI.md`, `.cursor/`, `.cursorrules` | none of the supported CLIs |

Because both sides use the same CLI and the same copy, they get exactly the same instructions.

## Adding a CLI (phase 2)

`codex` (OpenAI only) and `claude` (Anthropic only) will be new adapters with the same four fields:

1. a constructor like `opencodeAgent` that writes the CLI's config so its base URL is `http://api:4701/<provider>…` and its API key is the side token;
2. the install command (or a self-contained binary when the CLI offers one; Node copied from a multi-stage build when the runtime lacks it);
3. the autonomous and interactive command lines;
4. validation in `Service.Start`, and the CLI/provider filter in the UI.

The proxy needs no change for them: it already handles OpenAI and Anthropic formats. When the two sides use different CLIs, the UI will warn that they read different harness files.

## Things to verify for each new CLI

These were the spike's questions for opencode, all answered yes:

- Does it accept a custom base URL for its provider, so all traffic goes through the proxy?
- Does the proxy forward its API (streaming included) unchanged and find usage in it?
- Do models.dev model ids match the ids it accepts?
- Can its TUI start with an initial prompt, or tolerate one typed into the TTY?
- Does it have a non-interactive mode that runs to completion?
