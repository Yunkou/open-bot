"""In-process Laya. The package is imported only when this provider is used."""

from __future__ import annotations

import threading
from typing import Any

from .protocol import DecisionError, DecisionResult, normalize_questions, parse_result

LAYA_DEFAULT_MODEL = "convaiinnovations/laya"


class LayaLocalClient:
    def __init__(self, model: str) -> None:
        self.provider = "laya"
        self.enabled = True
        self.model = (model or LAYA_DEFAULT_MODEL).strip() or LAYA_DEFAULT_MODEL
        self._agent: Any = None
        self._lock = threading.Lock()

    def decide(self, state: Any, questions: dict[str, Any]) -> DecisionResult:
        prepared = normalize_questions(questions)
        agent = self._load()
        try:
            raw = agent.predict(state, prepared)
        except DecisionError:
            raise
        except Exception as exc:  # noqa: BLE001 — vendor errors stay inside the adapter
            raise DecisionError(f"Laya 推理失败: {exc}") from exc
        if not isinstance(raw, dict):
            raise DecisionError("Laya 返回了无法识别的结果")
        payload = raw if isinstance(raw.get("answers"), dict) else {"answers": raw}
        if not payload.get("model"):
            payload["model"] = self.model
        return parse_result("laya", payload, fallback_model=self.model)

    def _load(self) -> Any:
        with self._lock:
            if self._agent is not None:
                return self._agent
            try:
                import laya
            except ImportError as exc:
                raise DecisionError(
                    "未安装 laya。在 agent-runtime 执行 pip install laya，或改为填写 Laya 的 HTTP 地址。"
                ) from exc
            try:
                self._agent = laya.load(self.model)
            except Exception as exc:  # noqa: BLE001
                raise DecisionError(f"Laya 模型加载失败: {exc}") from exc
            return self._agent
