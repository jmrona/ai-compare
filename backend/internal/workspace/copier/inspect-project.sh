#!/bin/sh
# Describes a project without copying it: what would be copied, its size, the harness files it
# has and the files that hint at its toolchain. Same mount layout as copy-project.
# Prints one JSON line on stdout.
set -eu
set -o pipefail

src="/host/${1:-}"
list=/tmp/files

if [ ! -d "$src" ]; then
	echo "not a folder: $src" >&2
	exit 3
fi
if [ ! -r "$src" ] || [ ! -x "$src" ]; then
	echo "cannot read: $src" >&2
	exit 4
fi
cd "$src"

if git -c safe.directory='*' rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	git_repo=true
	git -c safe.directory='*' -c core.fsmonitor=false -c core.quotepath=off \
		ls-files -z --cached --others --exclude-standard >"$list.all"
else
	git_repo=false
	find . \( -name .git -o -name node_modules -o -name dist -o -name build -o -name .venv \
		-o -name venv -o -name target -o -name __pycache__ \) -prune \
		-o \( -type f -o -type l \) -printf '%P\0' >"$list.all"
fi

env_re='(^|/)\.env(\.[^/]*)?$'
template_re='(^|/)\.env\.(example|sample|template|dist)$'
{ grep -zvE "$env_re" "$list.all" || true; grep -zE "$template_re" "$list.all" || true; } >"$list"

files=$(tr -cd '\0' <"$list" | wc -c | tr -d ' ')
bytes=$(du -cb --files0-from="$list" 2>/dev/null | tail -1 | cut -f1)

# JSON array of the given names that exist at the project root.
present() {
	out=""
	for name in "$@"; do
		if [ -e "$name" ]; then
			out="$out${out:+,}\"$name\""
		fi
	done
	printf '[%s]' "$out"
}

env_files=$(grep -zE "$env_re" "$list.all" | grep -zvE "$template_re" | tr '\0' '\n' | sed 's/.*/"&"/' | paste -sd, - || true)

printf '{"git":%s,"files":%s,"bytes":%s,"harness":%s,"markers":%s,"envFiles":[%s]}\n' \
	"$git_repo" "$files" "${bytes:-0}" \
	"$(present AGENTS.md CLAUDE.md CLAUDE.local.md GEMINI.md .claude .agents .codex .opencode opencode.json opencode.jsonc .mcp.json .cursor .cursorrules)" \
	"$(present package.json package-lock.json pnpm-lock.yaml yarn.lock bun.lockb requirements.txt pyproject.toml go.mod Cargo.toml)" \
	"$env_files"
