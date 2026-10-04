# Working in This Project

This project is set up with the elelem rules and skills for opencode, and with the codebase-memory MCP server. They were installed with elelem's opencode installer, so they already use opencode's tool names. This file says what is loaded and when, and what this project adds. The rules themselves are the canonical source; this file does not repeat them.

## The Load Model

| File class                                | In context when                                                      |
|-------------------------------------------|----------------------------------------------------------------------|
| `.opencode/AGENTS.md` (this file)         | Always: listed under `instructions` in `.opencode/opencode.json`     |
| `.opencode/rules/common/*.md`             | Always: listed under `instructions` in `.opencode/opencode.json`     |
| `.opencode/rules/<lang>/*.md`             | You read it before working on a file its `globs:` frontmatter matches |
| `.opencode/skills/<name>/SKILL.md`        | That skill is loaded with the `skill` tool                           |
| Sibling files in a skill folder           | `SKILL.md` instructs a read, or launches a script file               |

opencode does not load language rules by their `globs:` on its own. Before you write or review a file in one of these languages, you **MUST** read that language's rules first: `go`, `javascript`, `php`, `python`, `rust`, `typescript`, under `.opencode/rules/<lang>/`.

## Notes for opencode

- A subagent dispatched with the `task` tool runs in a child session. The built-in agent types are `general` (can change files) and `explore` (reads and searches only).
- Plan mode is toggled by the user with Tab; you cannot enter it yourself.
- A cross-reference such as `../../rules/common/debugging.md` resolves from the citing file's own folder inside `.opencode/`.

## Choosing a Model for a Subagent

Append an alias to `subagent_type` to run the subagent on a model that fits the work, for example `task(subagent_type: "explore@luna", …)`. These are the only model identifiers this environment confirms; never write any other.

| Alias        | Model                       | Use it for                                                                   |
|--------------|-----------------------------|------------------------------------------------------------------------------|
| `@luna`      | gpt-6-luna, low effort      | Lookups, searches, renames, formatting, mechanical edits                     |
| `@luna-high` | gpt-6-luna, high effort     | Ordinary implementation, focused reviews and tests: the default for most work |
| `@sol`       | gpt-6.1-sol, high effort    | Design, deep reasoning, debugging across modules, security-sensitive reviews |

Without an alias, a subagent runs on the main agent's model. A skill whose `SKILL.md` declares `metadata.model` runs on that model in its own child session; the other skills run in this conversation.

## Finding Your Way Around the Code

The `codebase-memory` MCP server keeps a knowledge graph of this project. Prefer it to reading many files: it answers structural questions with far fewer tokens.

1. At the start of a task, call `codebase-memory_index_status`; if the project is not indexed or the index is stale, call `codebase-memory_index_repository` on the project root.
2. Get the lay of the land with `codebase-memory_get_architecture`, then find symbols with `codebase-memory_search_graph` or `codebase-memory_search_code`.
3. Before you change a function, use `codebase-memory_trace_path` to see its callers and callees, and `codebase-memory_get_code_snippet` or `codebase-memory_get_file_outline` to read only what you need.
4. Before you finish, `codebase-memory_detect_changes` maps your diff to the code it affects; check that nothing affected was left behind.

Read whole files only when the graph cannot answer, or when you are about to edit them.

## Unattended Runs

When the task says you are running unattended, `.opencode/rules/common/unattended.md` overrides every instruction to ask, wait for approval or consult: you make those choices yourself and list them in your final message.

## Finishing

The rules on verification and on git apply to every change. In short: run the checks the rules name for the files you changed, and state in your final message what you ran, what passed and anything you could not do.
