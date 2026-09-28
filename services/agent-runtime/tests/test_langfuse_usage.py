"""Unit tests for Langfuse usage_details parsing/merging (no live Langfuse)."""

from __future__ import annotations

import sys
from pathlib import Path
from types import SimpleNamespace

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app import langfuse_trace as lf  # noqa: E402


def _ok(cond: bool, msg: str) -> None:
    if not cond:
        raise AssertionError(msg)
    print(f"  OK  {msg}")


def test_parse_standard_openai() -> None:
    d = lf.parse_usage_details(
        {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
    )
    _ok(d == {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}, f"standard: {d}")


def test_parse_aliases_input_output() -> None:
    d = lf.parse_usage_details({"input_tokens": 3, "output_tokens": 7})
    _ok(
        d == {"prompt_tokens": 3, "completion_tokens": 7, "total_tokens": 10},
        f"aliases input/output: {d}",
    )


def test_parse_camel_case() -> None:
    d = lf.parse_usage_details(
        {"promptTokens": 2, "completionTokens": 4, "totalTokens": 6}
    )
    _ok(
        d == {"prompt_tokens": 2, "completion_tokens": 4, "total_tokens": 6},
        f"camelCase: {d}",
    )


def test_parse_object_attrs() -> None:
    obj = SimpleNamespace(prompt_tokens=11, completion_tokens=22, total_tokens=33)
    d = lf.parse_usage_details(obj)
    _ok(
        d == {"prompt_tokens": 11, "completion_tokens": 22, "total_tokens": 33},
        f"object: {d}",
    )


def test_parse_none_and_empty() -> None:
    _ok(lf.parse_usage_details(None) is None, "None → None")
    _ok(lf.parse_usage_details({}) is None, "empty dict → None")
    _ok(lf.parse_usage_details({"foo": 1}) is None, "unknown keys only → None")


def test_parse_partial_no_fake_zeros() -> None:
    d = lf.parse_usage_details({"prompt_tokens": 9})
    _ok(d == {"prompt_tokens": 9}, f"partial prompt only (no fake completion=0): {d}")
    _ok("completion_tokens" not in d, "no invented completion_tokens")
    _ok("total_tokens" not in d, "no invented total when only one side")


def test_parse_zero_is_real() -> None:
    # Explicit zeros from upstream are real data (e.g. empty completion) — keep them.
    d = lf.parse_usage_details(
        {"prompt_tokens": 5, "completion_tokens": 0, "total_tokens": 5}
    )
    _ok(
        d == {"prompt_tokens": 5, "completion_tokens": 0, "total_tokens": 5},
        f"explicit zeros preserved: {d}",
    )


def test_merge_usage() -> None:
    a = {"prompt_tokens": 10, "completion_tokens": 1, "total_tokens": 11}
    b = {"prompt_tokens": 20, "completion_tokens": 3, "total_tokens": 23}
    m = lf.merge_usage_details(a, b, None)
    _ok(
        m == {"prompt_tokens": 30, "completion_tokens": 4, "total_tokens": 34},
        f"merge: {m}",
    )
    _ok(lf.merge_usage_details(None, None) is None, "merge all None → None")
    _ok(lf.merge_usage_details(None, a) == a, "merge None + a → a")


def test_safe_update_passes_usage_details() -> None:
    class Obs:
        def __init__(self) -> None:
            self.kwargs = None

        def update(self, **kwargs):  # noqa: ANN003
            self.kwargs = kwargs

    obs = Obs()
    details = {"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3}
    lf.update_obs(obs, output="hi", usage_details=details, model="m1")
    _ok(obs.kwargs is not None, "update called")
    assert obs.kwargs is not None
    _ok(obs.kwargs.get("usage_details") == details, f"usage passed: {obs.kwargs}")
    _ok(obs.kwargs.get("model") == "m1", "model kept")
    _ok(obs.kwargs.get("output") == "hi", "output kept")


def test_noop_update_soft_fail() -> None:
    # Should not raise when tracing disabled / noop
    with lf.observation_generation(name="t", model="m") as obs:
        lf.update_obs(
            obs,
            output="x",
            usage_details={"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
        )
    _ok(True, "noop generation accepts usage_details without error")


def main() -> None:
    print("test_langfuse_usage")
    # Ensure disabled client so observation_generation yields noop
    lf._client = None  # noqa: SLF001
    lf._init_attempted = True  # noqa: SLF001
    lf._disabled_reason = "test"  # noqa: SLF001

    test_parse_standard_openai()
    test_parse_aliases_input_output()
    test_parse_camel_case()
    test_parse_object_attrs()
    test_parse_none_and_empty()
    test_parse_partial_no_fake_zeros()
    test_parse_zero_is_real()
    test_merge_usage()
    test_safe_update_passes_usage_details()
    test_noop_update_soft_fail()
    print("all passed")


if __name__ == "__main__":
    main()
