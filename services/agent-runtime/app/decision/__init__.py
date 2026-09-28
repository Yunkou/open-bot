"""System One decision adapter.

Call sites use ``try_decide`` or ``current_decision()``. They do not import a
vendor SDK. ``provider=off`` (the default) never calls Jev or Laya.
"""

from .factory import bind_decision, build_client, current_decision, reset_decision, try_decide
from .protocol import DecisionDisabled, DecisionError, DecisionResult, DecisionSettings

__all__ = [
    "DecisionDisabled",
    "DecisionError",
    "DecisionResult",
    "DecisionSettings",
    "bind_decision",
    "build_client",
    "current_decision",
    "reset_decision",
    "try_decide",
]
