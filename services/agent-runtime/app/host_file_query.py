"""Narrow read-only host file summaries (largest / by extension / newest).

Builds a fixed bash pipeline and runs it through the same host shell path
(``op=shell``), so Auto-review / confirm / deny apply exactly like host_shell.
Never invents find -exec; never bypasses host exec review.
"""

from __future__ import annotations

import re
import shlex
from typing import Any

_SAFE_EXT = re.compile(r"^[A-Za-z0-9]{1,20}$")
_KIND_ALIASES = {
    "largest": "largest",
    "largest_by_size": "largest",
    "size": "largest",
    "by_ext": "by_ext",
    "largest_by_ext": "by_ext",
    "ext": "by_ext",
    "newest": "newest",
    "list_by_ext": "newest",
    "mtime": "newest",
}

def normalize_kind(raw: str) -> str:
    key = (raw or "").strip().lower().replace("-", "_")
    kind = _KIND_ALIASES.get(key)
    if not kind:
        raise ValueError(
            "query 须为 largest / by_ext / newest（或 largest_by_size / largest_by_ext / list_by_ext）"
        )
    return kind


def normalize_ext(raw: str, *, required: bool) -> str:
    ext = (raw or "").strip().lstrip(".")
    if not ext:
        if required:
            raise ValueError("按扩展名查询需要 ext（如 mp4）")
        return ""
    if not _SAFE_EXT.match(ext):
        raise ValueError("ext 只能是字母或数字（最多 20 字符）")
    return ext


def normalize_limit(raw: Any, default: int) -> int:
    try:
        n = int(raw) if raw is not None else default
    except (TypeError, ValueError):
        n = default
    return max(1, min(n, 50))


def normalize_dir(raw: str) -> str:
    """Return a directory path for bash $1; empty means $HOME/Downloads."""
    path = (raw or "").strip()
    if not path or path in ("~", "~/Downloads", "$HOME/Downloads", "${HOME}/Downloads"):
        return ""
    if "\n" in path or "\r" in path or "\x00" in path:
        raise ValueError("path 无效")
    return path


def build_command(
    *,
    query: str,
    path: str = "",
    ext: str = "",
    limit: int | None = None,
) -> str:
    """Return a read-only ``bash -c '…' -- dir [ext] n`` for host_shell."""
    kind = normalize_kind(query)
    directory = normalize_dir(path)
    if kind == "largest":
        n = normalize_limit(limit, 10)
        body = r"""
set -euo pipefail
DIR="${1:-$HOME/Downloads}"
N="${2:-10}"
if [[ ! -d "$DIR" ]]; then echo "not a directory: $DIR" >&2; exit 1; fi
if stat -f '%z' "$DIR" >/dev/null 2>&1; then
  find "$DIR" -type f -print0 2>/dev/null \
    | xargs -0 stat -f '%z %N' 2>/dev/null \
    | sort -nr | head -n "$N" \
    | awk '{ sz=$1; sub(/^[^ ]+ /,"",$0);
        if (sz>=1073741824) printf "%.1fGB\t%s\n", sz/1073741824, $0;
        else if (sz>=1048576) printf "%.1fMB\t%s\n", sz/1048576, $0;
        else if (sz>=1024) printf "%.1fKB\t%s\n", sz/1024, $0;
        else printf "%dB\t%s\n", sz, $0; }'
else
  find "$DIR" -type f -printf '%s %p\n' 2>/dev/null \
    | sort -nr | head -n "$N" \
    | awk '{ sz=$1; sub(/^[^ ]+ /,"",$0);
        if (sz>=1073741824) printf "%.1fGB\t%s\n", sz/1073741824, $0;
        else if (sz>=1048576) printf "%.1fMB\t%s\n", sz/1048576, $0;
        else if (sz>=1024) printf "%.1fKB\t%s\n", sz/1024, $0;
        else printf "%dB\t%s\n", sz, $0; }'
fi
""".strip()
        return f"bash -c {shlex.quote(body)} -- {shlex.quote(directory)} {n}"

    extension = normalize_ext(ext, required=True)
    if kind == "by_ext":
        n = normalize_limit(limit, 10)
        body = r"""
set -euo pipefail
DIR="${1:-$HOME/Downloads}"
EXT="${2}"
N="${3:-10}"
EXT="${EXT#.}"
if [[ ! -d "$DIR" ]]; then echo "not a directory: $DIR" >&2; exit 1; fi
if stat -f '%z' "$DIR" >/dev/null 2>&1; then
  find "$DIR" -type f -iname "*.${EXT}" -print0 2>/dev/null \
    | xargs -0 stat -f '%z %N' 2>/dev/null \
    | sort -nr | head -n "$N" \
    | awk '{ sz=$1; sub(/^[^ ]+ /,"",$0);
        if (sz>=1073741824) printf "%.1fGB\t%s\n", sz/1073741824, $0;
        else if (sz>=1048576) printf "%.1fMB\t%s\n", sz/1048576, $0;
        else if (sz>=1024) printf "%.1fKB\t%s\n", sz/1024, $0;
        else printf "%dB\t%s\n", sz, $0; }'
else
  find "$DIR" -type f -iname "*.${EXT}" -printf '%s %p\n' 2>/dev/null \
    | sort -nr | head -n "$N" \
    | awk '{ sz=$1; sub(/^[^ ]+ /,"",$0);
        if (sz>=1073741824) printf "%.1fGB\t%s\n", sz/1073741824, $0;
        else if (sz>=1048576) printf "%.1fMB\t%s\n", sz/1048576, $0;
        else if (sz>=1024) printf "%.1fKB\t%s\n", sz/1024, $0;
        else printf "%dB\t%s\n", sz, $0; }'
fi
""".strip()
        return (
            f"bash -c {shlex.quote(body)} -- "
            f"{shlex.quote(directory)} {shlex.quote(extension)} {n}"
        )

    # newest / list_by_ext
    n = normalize_limit(limit, 20)
    body = r"""
set -euo pipefail
DIR="${1:-$HOME/Downloads}"
EXT="${2}"
N="${3:-20}"
EXT="${EXT#.}"
if [[ ! -d "$DIR" ]]; then echo "not a directory: $DIR" >&2; exit 1; fi
if stat -f '%m' "$DIR" >/dev/null 2>&1; then
  find "$DIR" -type f -iname "*.${EXT}" -print0 2>/dev/null \
    | xargs -0 stat -f '%m %z %N' 2>/dev/null \
    | sort -nr | head -n "$N" \
    | awk '{ mt=$1; sz=$2; sub(/^[^ ]+ [^ ]+ /,"",$0);
        if (sz>=1073741824) s=sprintf("%.1fGB", sz/1073741824);
        else if (sz>=1048576) s=sprintf("%.1fMB", sz/1048576);
        else if (sz>=1024) s=sprintf("%.1fKB", sz/1024);
        else s=sprintf("%dB", sz);
        printf "%d\t%s\t%s\n", mt, s, $0; }'
else
  find "$DIR" -type f -iname "*.${EXT}" -printf '%T@ %s %p\n' 2>/dev/null \
    | sort -nr | head -n "$N" \
    | awk '{ mt=int($1); sz=$2; sub(/^[^ ]+ [^ ]+ /,"",$0);
        if (sz>=1073741824) s=sprintf("%.1fGB", sz/1073741824);
        else if (sz>=1048576) s=sprintf("%.1fMB", sz/1048576);
        else if (sz>=1024) s=sprintf("%.1fKB", sz/1024);
        else s=sprintf("%dB", sz);
        printf "%d\t%s\t%s\n", mt, s, $0; }'
fi
""".strip()
    return (
        f"bash -c {shlex.quote(body)} -- "
        f"{shlex.quote(directory)} {shlex.quote(extension)} {n}"
    )


HOST_FILE_QUERY_TOOL: dict[str, Any] = {
    "type": "function",
    "function": {
        "name": "host_file_query",
        "description": (
            "Read-only compact file summary on a connected computer: largest files, "
            "largest by extension, or newest by extension. Prefer this over inventing "
            "a long find/exec or dumping host_ls. Runs through the same host shell "
            "Auto-review path as host_shell (confirm/deny unchanged). "
            "query=largest|by_ext|newest; path defaults to ~/Downloads; "
            "ext required for by_ext/newest (e.g. mp4); limit max 50."
        ),
        "parameters": {
            "type": "object",
            "properties": {
                "machine_id": {
                    "type": "string",
                    "description": "Machine id from list_machines; omit for usual work computer",
                },
                "query": {
                    "type": "string",
                    "description": "largest | by_ext | newest",
                },
                "path": {
                    "type": "string",
                    "description": "Directory to search (default ~/Downloads)",
                },
                "ext": {
                    "type": "string",
                    "description": "Extension without dot (required for by_ext / newest)",
                },
                "limit": {
                    "type": "integer",
                    "description": "Max rows (default 10 for largest/by_ext, 20 for newest; max 50)",
                },
            },
            "required": ["query"],
        },
    },
}
