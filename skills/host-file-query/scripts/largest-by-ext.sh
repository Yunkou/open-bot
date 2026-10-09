#!/usr/bin/env bash
# Usage: largest-by-ext.sh [dir] [ext] [n]
# Print the N largest files matching *.<ext> under dir (default ~/Downloads, ext=mp4, n=10).
set -euo pipefail
DIR="${1:-$HOME/Downloads}"
EXT="${2:-mp4}"
N="${3:-10}"
EXT="${EXT#.}"
if [[ ! -d "$DIR" ]]; then
  echo "not a directory: $DIR" >&2
  exit 1
fi
if stat -f '%z' "$DIR" >/dev/null 2>&1; then
  find "$DIR" -type f -iname "*.${EXT}" -print0 2>/dev/null \
    | xargs -0 stat -f '%z %N' 2>/dev/null \
    | sort -nr \
    | head -n "$N" \
    | awk '{
        sz=$1; sub(/^[^ ]+ /,"",$0);
        if (sz>=1073741824) printf "%.1fGB\t%s\n", sz/1073741824, $0;
        else if (sz>=1048576) printf "%.1fMB\t%s\n", sz/1048576, $0;
        else if (sz>=1024) printf "%.1fKB\t%s\n", sz/1024, $0;
        else printf "%dB\t%s\n", sz, $0;
      }'
else
  find "$DIR" -type f -iname "*.${EXT}" -printf '%s %p\n' 2>/dev/null \
    | sort -nr \
    | head -n "$N" \
    | awk '{
        sz=$1; sub(/^[^ ]+ /,"",$0);
        if (sz>=1073741824) printf "%.1fGB\t%s\n", sz/1073741824, $0;
        else if (sz>=1048576) printf "%.1fMB\t%s\n", sz/1048576, $0;
        else if (sz>=1024) printf "%.1fKB\t%s\n", sz/1024, $0;
        else printf "%dB\t%s\n", sz, $0;
      }'
fi
