"""assemble_llm_messages + Langfuse generation input prepare."""

from __future__ import annotations

import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app.llm import assemble_llm_messages  # noqa: E402
from app import langfuse_trace as lf  # noqa: E402


def test_assemble_merges_dual_system() -> None:
    msgs = assemble_llm_messages(
        "PERSONA\n## env",
        [
            {"role": "system", "content": "[对话摘要] older turns"},
            {"role": "user", "content": "hi"},
            {"role": "assistant", "content": "hello"},
        ],
    )
    assert len(msgs) == 3
    assert msgs[0]["role"] == "system"
    assert msgs[0]["content"].startswith("PERSONA")
    assert "## 对话摘要（更早轮次）" in msgs[0]["content"]
    assert "[对话摘要] older turns" in msgs[0]["content"]
    assert sum(1 for m in msgs if m["role"] == "system") == 1
    assert msgs[1]["role"] == "user"


def test_assemble_no_summary() -> None:
    msgs = assemble_llm_messages("SYS", [{"role": "user", "content": "x"}])
    assert msgs == [{"role": "system", "content": "SYS"}, {"role": "user", "content": "x"}]


def test_prepare_shows_system_by_default() -> None:
    os.environ.pop("LANGFUSE_REDACT_SYSTEM", None)
    out = lf._prepare_generation_input(  # noqa: SLF001
        [
            {"role": "system", "content": "SECRET_PERSONA_TEXT"},
            {"role": "user", "content": "hi"},
        ]
    )
    assert isinstance(out, list)
    assert out[0]["role"] == "system"
    assert "SECRET_PERSONA_TEXT" in str(out[0]["content"])
    assert "omitted" not in str(out[0]["content"])


def test_prepare_redacts_when_enabled() -> None:
    os.environ["LANGFUSE_REDACT_SYSTEM"] = "1"
    try:
        out = lf._prepare_generation_input(  # noqa: SLF001
            [
                {"role": "system", "content": "SECRET_PERSONA_TEXT"},
                {"role": "user", "content": "hi"},
            ]
        )
        assert "omitted system prompt" in str(out[0]["content"])
        assert "SECRET_PERSONA_TEXT" not in str(out[0]["content"])
        assert out[1]["content"] == "hi" or "hi" in str(out[1]["content"])
    finally:
        os.environ.pop("LANGFUSE_REDACT_SYSTEM", None)


def main() -> None:
    print("test_assemble_and_langfuse_io")
    test_assemble_merges_dual_system()
    print("  OK  assemble merges")
    test_assemble_no_summary()
    print("  OK  assemble plain")
    test_prepare_shows_system_by_default()
    print("  OK  show system default")
    test_prepare_redacts_when_enabled()
    print("  OK  redact when enabled")
    print("all passed")


if __name__ == "__main__":
    main()
