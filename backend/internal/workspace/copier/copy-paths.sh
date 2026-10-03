#!/bin/sh
# Copies chosen files or folders of a host folder to /staging/<id>/<dest>, keeping their relative
# paths: what a preset imports from a project. Same mount layout as copy-project: /host is the
# top-level folder, read-only. $1 is the id, $2 the destination name, then the paths to copy,
# and the last argument is the folder's path inside /host. .env files are never copied.
# Prints one JSON line on stdout.
set -eu

id="$1"
dest_name="$2"
shift 2
for last; do :; done
src="/host/$last"

if [ ! -d "$src" ]; then
	echo "not a folder: $src" >&2
	exit 3
fi
if [ ! -r "$src" ] || [ ! -x "$src" ]; then
	echo "cannot read: $src" >&2
	exit 4
fi

# WSL creates symbolic links with absolute paths such as /mnt/c/Users/… or /mnt/host/c/Users/…,
# which do not resolve here. Replace those that point inside the mounted folder with a copy of
# their target (without .env files); other broken links stay as they are.
fix_wsl_links() { # $1: the destination folder, after copying
	find "$1" -type l 2>/dev/null | while IFS= read -r link; do
		rest=$(readlink "$link" | sed -n 's#^/mnt/\(host/\)\{0,1\}[a-zA-Z]/##p')
		if [ -z "$rest" ] || [ ! -e "/host/$rest" ]; then
			continue
		fi
		rm -f "$link"
		cp -R "/host/$rest" "$link"
		find "$link" \( -name .env -o -name '.env.*' \) ! -name .env.example ! -name .env.sample ! -name .env.template ! -name .env.dist -exec rm -f {} + 2>/dev/null
	done || true
}

dest="/staging/$id/$dest_name"
rm -rf "$dest"
mkdir -p "$dest"
cd "$src"

while [ $# -gt 1 ]; do
	p="${1%/}"
	shift
	case "$p" in
	/* | *..* | "") continue ;;
	esac
	[ -e "$p" ] || continue
	# Besides .env files, skip what CLIs keep for themselves rather than as configuration:
	# Claude Code's and Codex's worktrees (whole copies of the project), dependencies, history and locks.
	tar -c --exclude='.env' --exclude='.env.*' --exclude='.claude/worktrees' --exclude='.codex/worktrees' --exclude='node_modules' \
		--exclude='.git' --exclude='*.lock' --exclude='.codex/sessions' --exclude='.codex/log' \
		-f - "$p" | tar -x -C "$dest" -f -
done
fix_wsl_links "$dest"

files=$(find "$dest" -type f | wc -l | tr -d ' ')
printf '{"files":%s}\n' "$files"
