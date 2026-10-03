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
	tar -c --exclude='.env' --exclude='.env.*' -f - "$p" | tar -x -C "$dest" -f -
done

files=$(find "$dest" -type f | wc -l | tr -d ' ')
printf '{"files":%s}\n' "$files"
