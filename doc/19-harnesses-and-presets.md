# 19. Harnesses and presets

Code: `backend/internal/presets/` (the store), `backend/internal/rpc/preset.go` (`PresetService`), `backend/internal/workspace/` (how a harness reaches a side's image), `backend/internal/comparison/harness.go` (snapshots, the Harness tab, the adviser's input), `frontend/src/pages/HarnessesPage.tsx`, `frontend/src/components/compare/HarnessView.tsx`, `frontend/src/lib/presetCards.ts`.

## What a harness is

A **harness** is the set of files that instruct a coding agent, as opposed to the code it works on: `AGENTS.md`, `CLAUDE.md`, `.claude/`, `.agents/`, `.codex/`, `.opencode/`, `opencode.json`, `.mcp.json`, `.github/copilot-instructions.md` and the like. Two sides with the same model can behave very differently with different instructions, so the harness is part of a side's configuration.

Which CLI reads which file:

| File | Read by |
|---|---|
| `AGENTS.md` | opencode, codex |
| `CLAUDE.md`, `CLAUDE.local.md`, `.claude/`, `.mcp.json` | claude |
| `.opencode/`, `opencode.json`, `opencode.jsonc` | opencode |
| `.agents/`, `.codex/` | codex |
| `GEMINI.md`, `.cursor/`, `.cursorrules` | none of the supported CLIs |

## The three choices per side

Each side picks one (`SideConfig.harness`, chosen in the side form of a new comparison):

| Choice | What the side's image gets |
|---|---|
| **Project's harness** (default) | The project's harness files, copied as they are |
| **Preset** | The project's harness files left out at any depth, the preset's `project/` files added at the project root, and its `home/` files in the agent's home folder |
| **No harness** | The project's harness files left out, nothing added: the CLI and model out of the box |

All of this is applied while the side image is built, **before the baseline commit**, so the Changes tab shows only what the agent did. Giving each side a different preset is how two sets of instructions are compared; the same choice on both sides gives both exactly the same instructions.

## Presets

A preset is a reusable harness kept by ai-compare. They are managed on `/harnesses`:

- **Create** from scratch, from an **existing preset** (its files and notes are copied; the original is untouched), from **cards** (ready-made sections of `AGENTS.md`: small changes, tests with every change, read before writing, error handling, security, strict TypeScript, documentation, accessibility, performance, a final summary), and by **importing from a project**.
- **Import from a project:** the usual harness files of the chosen folder are preselected, and any other entry at its root can be added (for instructions kept in folders such as `rules/` or `skills/`). `copy-paths.sh` copies them read-only through the copy helper. Never imported: `.env` files, Claude Code's and Codex's worktrees, `node_modules`, `.git`, lock files, Codex sessions and logs. Symbolic links created in WSL are replaced by their targets.
- **Edit** in the browser: a collapsible file tree for `project/` and `home/`, Markdown rendering, an editor that checks JSON and TOML before saving, new file, rename or move, delete, files and folders dropped from the file manager or chosen with **choose files** and **choose a folder** (`.env` files are skipped; files dragged from an editor such as VS Code reach the browser without their content, and the page says so), details and notes, duplicate, delete, and a link to the history filtered by the preset. Values that look like API keys produce a warning: secrets belong in `.env`, never in a preset.

## Plugins, agents and opencode settings in a preset

A preset can carry a whole `.opencode/` folder in `project/`, and it applies as it would in your own project:

```
project/
  AGENTS.md                         read by opencode from the project root
  .opencode/
    opencode.json                   plugins, instructions, agents, permissions…
    package.json                    dependencies of local plugins, installed when the image is built
    agents/review.md                agents, each with its own model
    skills/tdd/SKILL.md             skills
    rules/testing.md                any instruction files, listed under "instructions"
    vendor/subagent-model-alias/    local plugins registered in opencode.json
    plugins/notify.ts               plugins loaded automatically
```

For example, `.opencode/opencode.json`:

```json
{
  "instructions": [".opencode/rules/*.md"],
  "plugin": [["./vendor/subagent-model-alias", { "models": [{ "name": "luna", "model": "openai/gpt-6-luna", "when": "Fast mechanical tasks." }] }]]
}
```

What a harness cannot change: the side's model, its reasoning effort and where requests go. ai-compare sets them in opencode's managed configuration, which wins over every other file. Plugins and agents can still send subagents to other models, of OpenAI or Anthropic: those requests go through the proxy too and are priced with their own model's price. Models other providers serve are not reachable from a side.

## Presets that ship with the app

`backend/internal/presets/defaults/<slug>/` holds presets in the same layout as the data volume. They are embedded in the binary and, when `api` starts, each one not added before is copied into the data volume (`Store.Seed`); `harnesses/.defaults` records which were added, so a default you delete or edit is never brought back or overwritten.

| Preset | What it carries |
|---|---|
| **Elelem · opencode** (`elelem-opencode`) | The rules and skills of the elelem repository as its opencode installer writes them (tool names for opencode), plus `rules/common/unattended.md`, in `.opencode/`; `AGENTS.md` and `rules/common` loaded through `instructions`; the plugin [opencode-subagent-model-alias](https://github.com/futureplc/opencode-subagent-model-alias) in `.opencode/vendor/` with three aliases (`@luna`: gpt-6-luna, low effort; `@luna-high`: gpt-6-luna, high effort; `@sol`: gpt-6.1-sol, high effort); the plugin [skill-model-router](https://github.com/futureplc/opencode-skill-model-router-plugin) in `.opencode/plugins/`, exported from the barrel file `plugins/index.ts` |
| **Elelem · opencode + codebase memory** (`elelem-opencode-memory`) | The same, plus the MCP server [codebase-memory-mcp](https://github.com/DeusData/codebase-memory-mcp), declared in `.opencode/package.json` (its binary is downloaded while the side image is built) and registered under `mcp` |

Both carry `.opencode/rules/common/unattended.md`, which says how the rules and skills apply when nobody can answer: no questions, the committee design skill above the design threshold and the skip path below it, self-approval, and every such decision listed at the end. The plugins and the MCP server are MIT licensed; their licences are kept next to their code. To change what ships, edit the files under `defaults/`: existing installations keep their copy, new ones get the new version.

## Where presets are stored

Presets are plain files in the Docker volume **`ai-compare_appdata`**, mounted in the `api` container at **`/data/app`** (`DATA_DIR`). Nothing about them is in Postgres.

```
/data/app/harnesses/
  strict-backend/            the slug: derived from the title when the preset is created, never changed
    preset.md                frontmatter (title, description, clis) and free notes
    project/                 a literal mirror of what goes to the project root
      AGENTS.md
      .claude/settings.json
      rules/common/testing.md
    home/                    what goes to the agent's home folder
      .codex/config.toml
```

`preset.md` looks like this:

```markdown
---
title: Strict backend
description: Small, tested changes
clis: [opencode]
---
Free notes about the preset.
```

The store reads the disk on every request, with no cache, so files added or changed directly in the volume show up on the next page load.

**Hash.** Every preset has a short hash of the paths and contents of its files. A comparison records the title and hash each side ran with, so the history can tell two versions of the same preset apart.

## What a comparison keeps

Editing or deleting a preset never changes what explains an old result, because each comparison keeps its own copies:

| Where | What | Lifetime |
|---|---|---|
| Staging volume, `<id>/preset-<side>/` | The preset copy used to build the side image | Removed by retention with the project copy |
| Artefacts volume, `<id>/<side>/preset/` | The same copy (`preset.md`, `project/`, `home/`), for the history | Kept unless retention is set to remove artefacts |
| Artefacts volume, `<id>/<side>/harness/` | For **Project's harness**: the project's harness files as copied | Same as above |

The **Harness** tab of a run or a history entry (`ComparisonService.GetHarness`) shows these files read-only. If the agent changed harness files anyway, the tab says so and shows their diff. When the two sides ran with different harnesses, the report's **harness adviser** reads both and lists the differences that may have mattered and what to try (see [Comparison lifecycle](04-comparison-lifecycle.md)).

## Looking at presets with Docker

From the repo root (in Git Bash on Windows, prefix the commands with `MSYS_NO_PATHCONV=1`):

```bash
docker compose exec api ls /data/app/harnesses
```

```bash
docker compose exec api find /data/app/harnesses/strict-backend -type f
```

```bash
docker compose exec api cat /data/app/harnesses/strict-backend/project/AGENTS.md
```

Copy every preset to the host, for a backup or to edit them in an editor:

```bash
docker compose cp api:/data/app/harnesses ./harnesses-backup
```

Put a preset folder back, or add one written by hand (it needs a `preset.md` with at least a title):

```bash
docker compose cp ./harnesses-backup/strict-backend api:/data/app/harnesses/strict-backend
```

Without the stack running, any small image can read the volume:

```bash
docker run --rm -v ai-compare_appdata:/data alpine find /data/harnesses -maxdepth 2
```

The snapshots of a comparison are in the artefacts volume:

```bash
docker compose exec api ls /data/artifacts/ra3f80e/A/preset /data/artifacts/ra3f80e/A/harness
```

In Docker Desktop the same files are under **Volumes → ai-compare_appdata → harnesses** (and **ai-compare_artifacts** for the snapshots).

`docker compose down -v` deletes the volumes, presets included; `docker compose down` keeps them.
