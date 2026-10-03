#!/bin/sh
# Lists the sub-folders of a folder for the folder browser. Same mount layout as copy-project:
# /host is the top-level folder, read-only, and $1 is the path inside it.
# Hidden folders and symbolic links are left out. Prints one JSON line on stdout.
set -eu

src="/host/${1:-}"

if [ ! -d "$src" ]; then
	echo "not a folder: $src" >&2
	exit 3
fi
if [ ! -r "$src" ] || [ ! -x "$src" ]; then
	echo "cannot read: $src" >&2
	exit 4
fi
cd "$src"

out=""
for d in *; do
	[ -d "$d" ] || continue
	[ -L "$d" ] && continue
	git=false
	[ -e "$d/.git" ] && git=true
	# Escape backslashes and double quotes for JSON.
	name=$(printf '%s' "$d" | sed 's/\\/\\\\/g; s/"/\\"/g')
	out="$out${out:+,}{\"name\":\"$name\",\"git\":$git}"
done
printf '{"folders":[%s]}\n' "$out"
