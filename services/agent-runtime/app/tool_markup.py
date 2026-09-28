"""Parse / strip XML & Hermes-style tool calls leaked into assistant text.

Some models (esp. local vLLM / Qwen without proper tool-choice) emit raw markup
like ``<tool_call><function=sandbox_ls>...</function></tool_call>`` as chat
content instead of OpenAI ``tool_calls``. When tools are enabled we recover
those into the normal dispatch path; either way we strip them from user-visible
replies.
"""

from __future__ import annotations

import json
import re
import uuid
from dataclasses import dataclass, field
from typing import Any


@dataclass
class ParsedToolCall:
    name: str
    arguments: dict[str, Any] = field(default_factory=dict)
    raw: str = ""


# Outer <tool_call>...</tool_call> (non-greedy, DOTALL)
_TOOL_CALL_BLOCK = re.compile(
    r"<tool_call\b[^>]*>\s*([\s\S]*?)\s*</tool_call\s*>",
    re.IGNORECASE,
)

# Bare <function=NAME>...</function> (also used inside tool_call)
_FUNCTION_EQ_BLOCK = re.compile(
    r"<function\s*=\s*([A-Za-z0-9_.\-]+)\s*>\s*([\s\S]*?)\s*</function\s*>",
    re.IGNORECASE,
)

# <parameter=KEY>VAL</parameter>  OR  <parameter name="KEY">VAL</parameter>
_PARAM_EQ = re.compile(
    r"<parameter\s*=\s*([A-Za-z0-9_.\-]+)\s*>\s*([\s\S]*?)\s*</parameter\s*>",
    re.IGNORECASE,
)
_PARAM_NAME_ATTR = re.compile(
    r"<parameter\b[^>]*\bname\s*=\s*[\"']([^\"']+)[\"'][^>]*>\s*([\s\S]*?)\s*</parameter\s*>",
    re.IGNORECASE,
)

# <invoke name="...">...</invoke>
_INVOKE_BLOCK = re.compile(
    r"<invoke\b[^>]*\bname\s*=\s*[\"']([^\"']+)[\"'][^>]*>\s*([\s\S]*?)\s*</invoke\s*>",
    re.IGNORECASE,
)

# Fenced ```xml / ```tool_call / ```tool ... ```
_FENCE = re.compile(
    r"```(?:xml|tool_call|tool|hermes)\s*\n([\s\S]*?)```",
    re.IGNORECASE,
)

def _coerce_value(raw: str) -> Any:
    s = (raw or "").strip()
    if not s:
        return ""
    if (s.startswith("{") and s.endswith("}")) or (s.startswith("[") and s.endswith("]")):
        try:
            return json.loads(s)
        except json.JSONDecodeError:
            pass
    low = s.lower()
    if low == "true":
        return True
    if low == "false":
        return False
    if low == "null":
        return None
    # int / float (no leading zero ambiguity beyond plain ints)
    try:
        if re.fullmatch(r"-?\d+", s):
            return int(s)
        if re.fullmatch(r"-?\d+\.\d+", s):
            return float(s)
    except ValueError:
        pass
    return s


def _params_from_inner(inner: str) -> dict[str, Any]:
    args: dict[str, Any] = {}
    for m in _PARAM_EQ.finditer(inner or ""):
        args[m.group(1)] = _coerce_value(m.group(2))
    for m in _PARAM_NAME_ATTR.finditer(inner or ""):
        key = m.group(1)
        if key not in args:
            args[key] = _coerce_value(m.group(2))
    return args


def _parse_json_tool_body(body: str) -> ParsedToolCall | None:
    """Hermes-ish JSON inside <tool_call>: {name, arguments|parameters|args}."""
    text = (body or "").strip()
    if not text:
        return None
    # Allow a single JSON object, possibly surrounded by noise
    start = text.find("{")
    end = text.rfind("}")
    if start < 0 or end <= start:
        return None
    try:
        obj = json.loads(text[start : end + 1])
    except json.JSONDecodeError:
        return None
    if not isinstance(obj, dict):
        return None
    name: Any = obj.get("name")
    fn = obj.get("function")
    if not name:
        if isinstance(fn, str):
            name = fn
        elif isinstance(fn, dict):
            name = fn.get("name")
    name = str(name or "").strip()
    if not name:
        return None
    args: Any = (
        obj.get("arguments")
        or obj.get("parameters")
        or obj.get("args")
        or obj.get("input")
        or {}
    )
    if isinstance(fn, dict) and not args:
        args = fn.get("arguments") or fn.get("parameters") or {}
    if isinstance(args, str):
        try:
            args = json.loads(args)
        except json.JSONDecodeError:
            args = {"_raw": args}
    if not isinstance(args, dict):
        args = {"value": args}
    return ParsedToolCall(name=name, arguments=dict(args), raw=text)


def _parse_function_eq_blocks(text: str) -> list[ParsedToolCall]:
    out: list[ParsedToolCall] = []
    for m in _FUNCTION_EQ_BLOCK.finditer(text or ""):
        name = m.group(1).strip()
        inner = m.group(2) or ""
        if not name:
            continue
        out.append(
            ParsedToolCall(
                name=name,
                arguments=_params_from_inner(inner),
                raw=m.group(0),
            )
        )
    return out


def _parse_invoke_blocks(text: str) -> list[ParsedToolCall]:
    out: list[ParsedToolCall] = []
    for m in _INVOKE_BLOCK.finditer(text or ""):
        name = m.group(1).strip()
        if not name:
            continue
        out.append(
            ParsedToolCall(
                name=name,
                arguments=_params_from_inner(m.group(2) or ""),
                raw=m.group(0),
            )
        )
    return out


def _parse_one_tool_call_inner(inner: str, raw_block: str) -> list[ParsedToolCall]:
    """Parse the inside of a single <tool_call>...</tool_call>."""
    # Prefer XML function= / invoke shapes first (matches the reported bug)
    found = _parse_function_eq_blocks(inner)
    if found:
        for p in found:
            p.raw = raw_block
        return found
    found = _parse_invoke_blocks(inner)
    if found:
        for p in found:
            p.raw = raw_block
        return found
    js = _parse_json_tool_body(inner)
    if js:
        js.raw = raw_block
        return [js]
    # Hermes line form: first non-empty line = name, then key=value / key: value
    lines = [ln.strip() for ln in (inner or "").splitlines() if ln.strip()]
    if lines:
        name = lines[0]
        # Reject if first line looks like markup
        if not re.match(r"^[A-Za-z0-9_.\-]+$", name):
            return []
        args: dict[str, Any] = {}
        for ln in lines[1:]:
            m = re.match(r"^([A-Za-z0-9_.\-]+)\s*[:=]\s*(.*)$", ln)
            if m:
                args[m.group(1)] = _coerce_value(m.group(2))
        return [ParsedToolCall(name=name, arguments=args, raw=raw_block)]
    return []


def content_has_tool_markup(text: str) -> bool:
    if not text:
        return False
    if _TOOL_CALL_BLOCK.search(text):
        return True
    if _FUNCTION_EQ_BLOCK.search(text):
        return True
    if _INVOKE_BLOCK.search(text):
        return True
    if _FENCE.search(text):
        return True
    low = text.lower()
    return "<tool_call" in low or "<function=" in low


def parse_tool_markup(text: str) -> list[ParsedToolCall]:
    """Extract tool name + args from leaked XML / Hermes markup in ``text``."""
    if not text or not content_has_tool_markup(text):
        return []

    # Expand fenced blocks into the scan corpus (keep original too)
    corpus = text
    for m in _FENCE.finditer(text):
        corpus = corpus + "\n" + m.group(1)

    results: list[ParsedToolCall] = []
    seen_spans: set[str] = set()

    for m in _TOOL_CALL_BLOCK.finditer(corpus):
        raw = m.group(0)
        if raw in seen_spans:
            continue
        seen_spans.add(raw)
        results.extend(_parse_one_tool_call_inner(m.group(1), raw))

    # Bare <function=...> not already covered
    for p in _parse_function_eq_blocks(corpus):
        if any(p.raw and p.raw in (r.raw or "") for r in results):
            continue
        # Skip if this span sits inside an already-captured tool_call
        if any(p.raw and p.raw in span for span in seen_spans):
            continue
        results.append(p)

    for p in _parse_invoke_blocks(corpus):
        if any(p.raw and p.raw in (r.raw or "") for r in results):
            continue
        if any(p.raw and p.raw in span for span in seen_spans):
            continue
        results.append(p)

    return [r for r in results if r.name]


def strip_tool_markup(text: str) -> str:
    """Remove tool-call XML / fences from text so they never reach the user bubble."""
    if not text:
        return text
    out = text
    # Fences first (may contain inner tool_call)
    out = _FENCE.sub("", out)
    out = _TOOL_CALL_BLOCK.sub("", out)
    out = _FUNCTION_EQ_BLOCK.sub("", out)
    out = _INVOKE_BLOCK.sub("", out)
    # Unclosed leftovers
    out = re.sub(r"<tool_call\b[^>]*>[\s\S]*$", "", out, flags=re.IGNORECASE)
    out = re.sub(r"<function\s*=\s*[^>]+>[\s\S]*$", "", out, flags=re.IGNORECASE)
    # Collapse excess blank lines from stripping
    out = re.sub(r"\n{3,}", "\n\n", out)
    return out.strip()


def to_openai_tool_calls(parsed: list[ParsedToolCall]) -> list[dict[str, Any]]:
    """Convert parsed markup calls into OpenAI-shaped tool_calls dicts."""
    out: list[dict[str, Any]] = []
    for i, p in enumerate(parsed):
        tid = f"markup_{i}_{uuid.uuid4().hex[:8]}"
        out.append(
            {
                "id": tid,
                "type": "function",
                "function": {
                    "name": p.name,
                    "arguments": json.dumps(p.arguments or {}, ensure_ascii=False),
                },
            }
        )
    return out
