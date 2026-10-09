"""Postgres (or in-memory) checkpointer factory for LangGraph."""

from __future__ import annotations

import logging
from typing import Any

from .config import checkpoint_backend

logger = logging.getLogger("open-bot.harness.checkpointer")

_saver: Any | None = None
_pool: Any | None = None
_memory_saver: Any | None = None
_setup_done = False


def _database_url() -> str:
    from ..memory import database_url

    return database_url()


async def get_checkpointer() -> Any:
    """Return a process-wide checkpointer (Postgres preferred, Memory fallback)."""
    global _saver, _pool, _memory_saver, _setup_done

    backend = checkpoint_backend()
    if backend == "memory":
        if _memory_saver is None:
            from langgraph.checkpoint.memory import MemorySaver

            _memory_saver = MemorySaver()
        return _memory_saver

    if _saver is not None:
        return _saver

    try:
        from psycopg_pool import AsyncConnectionPool
        from langgraph.checkpoint.postgres.aio import AsyncPostgresSaver

        conninfo = _database_url()
        # LangGraph needs autocommit + dict row factory friendly connections.
        _pool = AsyncConnectionPool(
            conninfo=conninfo,
            kwargs={"autocommit": True, "prepare_threshold": 0},
            open=False,
            min_size=1,
            max_size=8,
        )
        await _pool.open()
        _saver = AsyncPostgresSaver(_pool)
        if not _setup_done:
            await _saver.setup()
            _setup_done = True
        logger.info("LangGraph Postgres checkpointer ready")
        return _saver
    except Exception as exc:  # noqa: BLE001
        logger.warning("Postgres checkpointer unavailable (%s); using MemorySaver", exc)
        if _memory_saver is None:
            from langgraph.checkpoint.memory import MemorySaver

            _memory_saver = MemorySaver()
        return _memory_saver


async def close_checkpointer() -> None:
    global _saver, _pool, _setup_done
    _saver = None
    _setup_done = False
    if _pool is not None:
        try:
            await _pool.close()
        except Exception:  # noqa: BLE001
            pass
        _pool = None
