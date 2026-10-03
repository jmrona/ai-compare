#!/bin/sh
# Collects a side's result once its agent has ended. Runs as root, without network, in an image
# committed from the side's container. $1 is the output folder in the artifacts volume.
#
# Writes:
#   workspace.tar                       the files the agent left (without .git and node_modules)
#   solution.diff, solution.numstat     changes against the baseline commit, harness files excluded
#   harness.diff, harness.numstat       changes to harness files only
#   session.json                        the CLI's sessions (opencode export), when available
set -u

# Root reads a repository owned by the agent's user: tell Git (and the CLI, which runs Git
# itself) that this is fine, or they treat it as a different project.
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0='*'

out="$1"
mkdir -p "$out"
cd /workspace

tar -c --exclude=./.git --exclude=./node_modules -f "$out/workspace.tar" .

g() { git -c safe.directory='*' -c core.quotepath=off "$@"; }

# Harness files at the project root, as in the copy and in the inspection.
include=""
exclude=""
for h in AGENTS.md CLAUDE.md CLAUDE.local.md GEMINI.md .claude .agents .codex .opencode opencode.json opencode.jsonc .mcp.json .cursor .cursorrules; do
	include="$include :(top,literal)$h"
	exclude="$exclude :(top,exclude,literal)$h"
done

if g rev-parse --verify -q baseline >/dev/null; then
	g add -A >/dev/null 2>&1
	# shellcheck disable=SC2086 # the pathspecs contain no spaces
	g diff --cached --numstat baseline -- . $exclude >"$out/solution.numstat"
	g diff --cached baseline -- . $exclude >"$out/solution.diff"
	g diff --cached --numstat baseline -- $include >"$out/harness.numstat"
	g diff --cached baseline -- $include >"$out/harness.diff"
fi

# The CLI keeps its sessions under the agent's home folder.
home=/root
[ -d /home/agent ] && home=/home/agent
if command -v opencode >/dev/null 2>&1; then
	ids=$(HOME="$home" opencode session list --format json 2>/dev/null | grep -o '"id": *"ses_[^"]*"' | sed 's/.*"\(ses_[^"]*\)"/\1/')
	{
		printf '['
		sep=""
		for id in $ids; do
			printf '%s' "$sep"
			# Keep only the JSON (the CLI may print a line before it).
			HOME="$home" opencode export "$id" 2>/dev/null | sed -n '/^{/,$p'
			sep=","
		done
		printf ']'
	} >"$out/session.json"
fi
echo done
