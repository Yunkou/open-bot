#!/usr/bin/env bash
# Usage: list-by-ext.sh [dir] [ext] [n]
# List up to N files matching *.<ext> under dir, newest mtime first.
# Compact lines: epoch_mtime\tSIZE\tPATH
set -euo pipefail
DIR="${1:-$HOME/Downloads}"
EXT="${2:-mp4}"
N="${3:-20}"
EXT="${EXT#.}"
if [[ ! -d "$DIR" ]]; then
  echo "not a directory: $DIR" >&2
  exit 1
fi
if stat -f '%m' "$DIR" >/dev/null 2>&1; then
  find "$DIR" -type f -iname "*.${EXT}" -print0 2>/dev/null \
    | xargs -0 stat -f '%m %z %N' 2>/dev/null \
    | sort -nr \
    | head -n "$N" \
    | awk '{
        mt=$1; sz=$2; sub(/^[^ ]+ [^ ]+ /,"",$0);
        if (sz>=1073741824) s=sprintf("%.1fGB", sz/1073741824);
        else if (sz>=1048576) s=sprintf("%.1fMB", sz/1048576);
        else if (sz>=1024) s=sprintf("%.1fKB", sz/1024);
        else s=sprintf("%dB", sz);
        printf "%d\t%s\t%s\n", mt, s, $0;
      }'
else
  find "$DIR" -type f -iname "*.${EXT}" -printf '%T@ %s %p\n' 2>/dev/null \
    | sort -nr \
    | head -n "$N" \
    | awk '{
        mt=int($1); sz=$2; sub(/^[^ ]+ [^ ]+ /,"",$0);
        if (sz>=1073741824) s=sprintf("%.1fGB", sz/1073741824);
        else if (sz>=1048576) s=sprintf("%.1fMB", sz/1048576);
        else if (sz>=1024) s=sprintf("%.1fKB", sz/1024);
        else s=sprintf("%dB", sz);
        printf "%d\t%s\t%s\n", mt, s, $0;
      }'
fi
