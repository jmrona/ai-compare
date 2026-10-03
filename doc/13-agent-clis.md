# 13. Agent CLIs

Code: `backend/internal/comparison/agent.go`.

An **agent adapter** answers four questions for a CLI:

| Question | Field | Example for opencode |
|---|---|---|
| How is it installed in the image? | `install` | `npm install -g opencode-ai@1.18.34 && npm cache clean --force` |
| Which files configure it? | `homeFiles` (copied to `/home/agent`, owned by `agent`, after the baseline commit) | `.config/opencode/opencode.json` |
| Which environment does it need? | `env` | `OPENAI_API_KEY=<side token>` (or `ANTHROPIC_API_KEY` for Anthropic) |
| What command starts it? | `command` | see below |

## opencode (OpenAI since phase 1, Anthropic since phase 2)

**Pinned version:** `1.18.34` (`OpencodeVersion`). Two comparisons on different days must run the same CLI, and the version is shown with each side. `autoupdate: false` stops it from updating itself.

Generated `/home/agent/.config/opencode/opencode.json` (the agent runs as the user `agent` with `HOME=/home/agent`):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "autoupdate": false,
  "share": "disabled",
  "model": "openai/gpt-5.4-mini",
  "small_model": "openai/gpt-5.4-mini",
  "agent": { "build": { "model": "openai/gpt-5.4-mini", "variant": "low" } },
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
- **`agent.build.variant`** carries the effort chosen in the UI, for every provider: opencode's model variants are the reasoning efforts models.dev lists. For OpenAI models `reasoningEffort` is also set, as before.
- **Providers:** `openai` and `anthropic`, opencode's built-in providers (`opencodeProviders` in `agent.go`). For Anthropic the provider key is `anthropic`, the base URL `http://api:4701/anthropic/v1` and the token goes in `ANTHROPIC_API_KEY`; the Anthropic SDK sends it as `x-api-key`, which the proxy replaces with the real key. Each side picks its own provider, so OpenAI can be compared with Anthropic.

Commands:

| Mode | Command | Behaviour |
|---|---|---|
| `autonomous` (default) | `opencode run --auto -m <provider>/<model> [--variant <effort>] <prompt>` | Runs to completion without asking for permission; the container is the safety boundary. The side ends when the CLI exits |
| `interactive` | `opencode --prompt <prompt>` | The TUI opens with the prompt already sent; the user keeps the conversation going from the browser terminal and ends the side with Finish |

Both sides default to autonomous so they finish on their own. Interactive sides stay open until the user finishes them, which is intended (the user may want to ask follow-up questions); the time spent waiting for the user is estimated as human wait and taken out of the agent time (see [Comparison lifecycle](04-comparison-lifecycle.md#timings-and-metrics)).

## Sessions: events and a usage cross-check

opencode keeps its sessions under the agent's home folder. After the agent stops, `collect-result.sh` (in a container of the side's result image, as root with `HOME=/home/agent` and Git's `safe.directory=*`, so opencode recognises the agent's project) runs `opencode session list --format json` and `opencode export <id>` for each session, and saves them as one JSON array, `session.json`, in the side's artefacts.

`comparison/timeline.go` reads it for the Events tab: the user's prompt, the agent's messages, each tool call with a summary of its main argument (command, file path, pattern, URL…; for patches, the files touched), `patch` parts with the files they changed, and failed tool calls as errors. It also adds up the tokens and cost the CLI recorded per message. Those figures are only a **cross-check**: the proxy's are the ones used, because the proxy also sees requests the CLI does not count (opencode's session titles, for example).

For a new CLI, the equivalent is: where it keeps its sessions, a command or file format to export them, and how to map its entries onto these event kinds.

## Harness files

Each side runs with one of three harnesses (`SideConfig.harness`):

| Choice | What the side's image gets |
|---|---|
| **Project's harness** (default) | The project's harness files, copied as they are |
| **Preset** | The project's harness files left out (at any depth), the preset's `project/` files added before the baseline commit and its `home/` files in the agent's home (ai-compare's own CLI configuration wins over a preset's) |
| **No harness** | The project's harness files left out, nothing added |

**Cards** (`frontend/src/lib/presetCards.ts`) are ready-made instruction blocks (small changes, tests with every change, read before writing, error handling, security, strict TypeScript, documentation, accessibility, performance, a final summary) that build or extend a preset's `project/AGENTS.md`, which opencode reads. Their versions for `CLAUDE.md` and the other CLIs come with those CLIs in phase 3.

Presets are managed on `/harnesses` and stored on disk (see [Project copy and side images](05-project-copy-and-images.md#presets)). The inspection tells the user which files a project has and which CLI reads each:

| File | Read by |
|---|---|
| `AGENTS.md` | opencode, codex |
| `CLAUDE.md`, `CLAUDE.local.md`, `.claude/`, `.mcp.json` | claude |
| `.opencode/`, `opencode.json`, `opencode.jsonc` | opencode |
| `.agents/`, `.codex/` | codex |
| `GEMINI.md`, `.cursor/`, `.cursorrules` | none of the supported CLIs |

With the same harness choice on both sides, they get exactly the same instructions; choosing a different preset per side is how two sets of instructions are compared.

## Adding a CLI (phase 3)

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
- Does it run as an unprivileged user with its configuration in that user's home folder?
- Can its session files be exported or read to build the Events tab?
