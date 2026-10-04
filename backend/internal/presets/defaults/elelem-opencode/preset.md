---
title: Elelem V1 · opencode
description: The elelem rules and skills for opencode, with subagents on a model that fits each task (luna, luna-high, sol) and skills routed to their own model.
clis: [opencode]
---
Built from the Elelem V1 harness, moved into .opencode/ so opencode reads it:

- .opencode/AGENTS.md says what is loaded and when, how the names the rules use (Agent, agent types) map onto opencode, and which subagent aliases exist. It and the always-on rules (rules/common) are loaded through "instructions" in .opencode/opencode.json. CLAUDE.md only imports AGENTS.md, for Claude Code. Language rules (rules/<lang>) are read before working on a file of that language: opencode has no globs for rules.
- Skills live in .opencode/skills.
- Plugin opencode-subagent-model-alias (MIT, Future Publishing) in .opencode/vendor, registered in .opencode/opencode.json: a subagent dispatched as type@luna, type@luna-high or type@sol runs on gpt-6-luna (low effort), gpt-6-luna (high effort) or gpt-6.1-sol.
- Plugin skill-model-router (MIT, Jose Romero and Future Publishing) in .opencode/plugins, exported from the barrel file .opencode/plugins/index.ts: a skill whose SKILL.md declares metadata.model runs in a child session on that model.
- .opencode/package.json declares the plugins' dependencies; ai-compare installs them while it builds the side image.
