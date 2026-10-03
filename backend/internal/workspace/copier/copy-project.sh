#!/bin/sh
# Copies the user's project to /staging/<id>/project.
#
# /host is a read-only mount of the top-level folder that contains the project (C:\, /Users,
# /home…) and $2 is the project path inside it. Mounting the project folder directly would make
# Docker Desktop create it on the host when it does not exist; this way a wrong path is reported.
#
# - Git repositories: tracked files plus untracked ones that .gitignore does not exclude,
#   i.e. the folder as it is, including uncommitted changes. Tracked files deleted from the
#   working tree are skipped.
# - Other folders: everything except .git and common dependency and build folders.
# - Never copied: .env files (their contents would be sent to the provider). Templates such
#   as .env.example, .env.sample and .env.template are kept.
#
# Prints one JSON line with the result on stdout.
set -eu
set -o pipefail

id="$1"
src="/host/${2:-}"
dest="/staging/$id/project"
list=/tmp/files

if [ ! -d "$src" ]; then
	echo "not a folder: $src" >&2
	exit 3
fi
if [ ! -r "$src" ] || [ ! -x "$src" ]; then
	echo "cannot read: $src" >&2
	exit 4
fi

rm -rf "$dest"
mkdir -p "$dest"
cd "$src"

if git -c safe.directory='*' rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	mode=git
	git -c safe.directory='*' -c core.fsmonitor=false -c core.quotepath=off \
		ls-files -z --cached --others --exclude-standard >"$list.all"
else
	mode=plain
	find . \( -name .git -o -name node_modules -o -name dist -o -name build -o -name .venv \
		-o -name venv -o -name target -o -name __pycache__ \) -prune \
		-o \( -type f -o -type l \) -printf '%P\0' >"$list.all"
fi

# Drop .env files, keep templates.
env_re='(^|/)\.env(\.[^/]*)?$'
template_re='(^|/)\.env\.(example|sample|template|dist)$'
{ grep -zvE "$env_re" "$list.all" || true; grep -zE "$template_re" "$list.all" || true; } >"$list"
secrets=$(grep -zE "$env_re" "$list.all" | grep -zvcE "$template_re" || true)

tar -c --null --no-recursion --ignore-failed-read --files-from="$list" -f - | tar -x -C "$dest" -f -

files=$(find "$dest" -type f | wc -l | tr -d ' ')
kilobytes=$(du -sk "$dest" | cut -f1)
printf '{"mode":"%s","files":%s,"kilobytes":%s,"envFilesSkipped":%s}\n' "$mode" "$files" "$kilobytes" "${secrets:-0}"
