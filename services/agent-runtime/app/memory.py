"""Postgres memory store (tiers: profile | log | note) with optional pgvector recall.

Falls back to per-user JSON files when DATABASE_URL is unreachable.
Falls back to keyword recall when embeddings / pgvector are unavailable.
"""

from __future__ import annotations

import json
import logging
import os
import threading
import uuid
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable

from .embedding import embed_one, embedding_config, vector_literal

logger = logging.getLogger(__name__)

TIERS = frozenset({"profile", "log", "note"})
DEFAULT_DATABASE_URL = "postgres://openbot:openbot@127.0.0.1:5432/openbot?sslmode=disable"


def _utcnow() -> str:
    return datetime.now(timezone.utc).isoformat()


def database_url() -> str:
    return (os.getenv("DATABASE_URL") or "").strip() or DEFAULT_DATABASE_URL


@dataclass
class MemoryItem:
    id: str
    tier: str
    content: str
    tags: list[str] = field(default_factory=list)
    created_at: str = field(default_factory=_utcnow)
    updated_at: str = field(default_factory=_utcnow)
    user_id: str | None = None
    has_embedding: bool = False


def default_store_path() -> Path:
    return Path(__file__).resolve().parents[1] / "data" / "memories.json"


def store_path_for_user(user_id: str | None) -> Path:
    base = Path(__file__).resolve().parents[1] / "data"
    if user_id and str(user_id).strip():
        safe = "".join(c if c.isalnum() or c in "-_" else "_" for c in str(user_id).strip())
        return base / "users" / f"{safe}.json"
    return default_store_path()


def re_split(q: str) -> list[str]:
    import re

    return re.split(r"[\s,，。；;、]+", q)


def _keyword_recall(candidates: list[MemoryItem], query: str, top_k: int) -> list[MemoryItem]:
    q = (query or "").strip().lower()
    if not candidates:
        return []
    if not q:
        return candidates[: max(1, top_k)]
    tokens = [t for t in re_split(q) if t]

    def score(item: MemoryItem) -> float:
        text = (item.content + " " + " ".join(item.tags)).lower()
        s = 0.0
        for t in tokens:
            if t in text:
                s += 1.0 + text.count(t) * 0.1
        if item.tier == "profile":
            s += 0.5
        return s

    ranked = sorted(candidates, key=score, reverse=True)
    out = [i for i in ranked if score(i) > 0]
    if not out:
        return ranked[: max(1, top_k)]
    return out[: max(1, top_k)]


class _JSONMemoryBackend:
    def __init__(self, path: Path) -> None:
        self.path = path
        self._lock = threading.RLock()
        self._items: list[MemoryItem] = []
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._load()

    @property
    def vector_enabled(self) -> bool:
        return False

    def _load(self) -> None:
        if not self.path.is_file():
            self._items = []
            return
        try:
            raw = json.loads(self.path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            self._items = []
            return
        items: list[MemoryItem] = []
        for row in raw if isinstance(raw, list) else raw.get("items", []):
            try:
                tier = str(row.get("tier", "note"))
                if tier not in TIERS:
                    tier = "note"
                items.append(
                    MemoryItem(
                        id=str(row.get("id") or uuid.uuid4()),
                        tier=tier,
                        content=str(row.get("content") or ""),
                        tags=list(row.get("tags") or []),
                        created_at=str(row.get("created_at") or _utcnow()),
                        updated_at=str(row.get("updated_at") or _utcnow()),
                    )
                )
            except Exception:  # noqa: BLE001
                continue
        self._items = [i for i in items if i.content.strip()]

    def _save(self) -> None:
        payload = [
            {k: v for k, v in asdict(i).items() if k not in ("user_id", "has_embedding")}
            for i in self._items
        ]
        self.path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")

    def write(
        self,
        content: str,
        tier: str = "note",
        tags: Iterable[str] | None = None,
        *,
        user_id: str | None = None,
    ) -> MemoryItem:
        content = (content or "").strip()
        if not content:
            raise ValueError("content required")
        tier = (tier or "note").strip().lower()
        if tier not in TIERS:
            raise ValueError(f"tier must be one of {sorted(TIERS)}")
        item = MemoryItem(
            id=str(uuid.uuid4()),
            tier=tier,
            content=content,
            tags=[t.strip() for t in (tags or []) if str(t).strip()],
            user_id=user_id,
        )
        with self._lock:
            self._items.append(item)
            self._save()
        return item

    def list_items(
        self,
        *,
        user_id: str | None = None,
        tier: str | None = None,
        limit: int = 50,
    ) -> list[MemoryItem]:
        with self._lock:
            items = list(self._items)
        if tier:
            items = [i for i in items if i.tier == tier.strip().lower()]
        items.sort(key=lambda x: x.updated_at, reverse=True)
        return items[: max(1, min(limit, 200))]

    def recall(
        self,
        query: str,
        *,
        user_id: str | None = None,
        tier: str | None = None,
        top_k: int = 5,
    ) -> list[MemoryItem]:
        with self._lock:
            candidates = list(self._items)
        if tier:
            candidates = [i for i in candidates if i.tier == tier.strip().lower()]
        return _keyword_recall(candidates, query, top_k)


class _PGMemoryBackend:
    def __init__(self, dsn: str) -> None:
        import psycopg

        self._dsn = dsn
        self._lock = threading.RLock()
        self._vector_enabled = False
        self._embedding_dim = embedding_config()[3]
        with psycopg.connect(dsn) as conn:
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS memories (
                  id TEXT PRIMARY KEY,
                  user_id TEXT NOT NULL,
                  tier TEXT NOT NULL DEFAULT 'note',
                  content TEXT NOT NULL DEFAULT '',
                  tags TEXT[] NOT NULL DEFAULT '{}',
                  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
                )
                """
            )
            try:
                conn.execute("CREATE EXTENSION IF NOT EXISTS vector")
                conn.execute(
                    f"ALTER TABLE memories ADD COLUMN IF NOT EXISTS embedding vector({self._embedding_dim})"
                )
                conn.commit()
                self._vector_enabled = True
            except Exception as e:  # noqa: BLE001
                conn.rollback()
                logger.warning("pgvector unavailable, keyword recall only: %s", e)
                self._vector_enabled = False

    @property
    def vector_enabled(self) -> bool:
        return self._vector_enabled

    def _connect(self):
        import psycopg

        return psycopg.connect(self._dsn)

    def write(
        self,
        content: str,
        tier: str = "note",
        tags: Iterable[str] | None = None,
        *,
        user_id: str | None = None,
    ) -> MemoryItem:
        content = (content or "").strip()
        if not content:
            raise ValueError("content required")
        tier = (tier or "note").strip().lower()
        if tier not in TIERS:
            raise ValueError(f"tier must be one of {sorted(TIERS)}")
        uid = (user_id or "").strip() or "_anonymous"
        tag_list = [t.strip() for t in (tags or []) if str(t).strip()]
        item_id = str(uuid.uuid4())
        emb = None
        if self._vector_enabled:
            emb = embed_one(content)
            if emb is not None and len(emb) != self._embedding_dim:
                logger.warning(
                    "embedding dim mismatch got=%s expected=%s; skipping",
                    len(emb),
                    self._embedding_dim,
                )
                emb = None
        with self._lock:
            with self._connect() as conn:
                if emb is not None:
                    row = conn.execute(
                        """
                        INSERT INTO memories (id, user_id, tier, content, tags, embedding, created_at, updated_at)
                        VALUES (%s, %s, %s, %s, %s, %s::vector, NOW(), NOW())
                        RETURNING id, tier, content, tags, created_at, updated_at, user_id, TRUE
                        """,
                        (item_id, uid, tier, content, tag_list, vector_literal(emb)),
                    ).fetchone()
                else:
                    row = conn.execute(
                        """
                        INSERT INTO memories (id, user_id, tier, content, tags, created_at, updated_at)
                        VALUES (%s, %s, %s, %s, %s, NOW(), NOW())
                        RETURNING id, tier, content, tags, created_at, updated_at, user_id, FALSE
                        """,
                        (item_id, uid, tier, content, tag_list),
                    ).fetchone()
                conn.commit()
        return self._row_to_item(row)

    def list_items(
        self,
        *,
        user_id: str | None = None,
        tier: str | None = None,
        limit: int = 50,
    ) -> list[MemoryItem]:
        uid = (user_id or "").strip() or "_anonymous"
        limit = max(1, min(int(limit or 50), 200))
        params: list[Any] = [uid]
        sql = """
            SELECT id, tier, content, tags, created_at, updated_at, user_id,
                   CASE WHEN embedding IS NULL THEN FALSE ELSE TRUE END
            FROM memories WHERE user_id = %s
        """
        if tier:
            sql += " AND tier = %s"
            params.append(tier.strip().lower())
        sql += " ORDER BY updated_at DESC LIMIT %s"
        params.append(limit)
        try:
            with self._connect() as conn:
                rows = conn.execute(sql, params).fetchall()
        except Exception:  # noqa: BLE001
            sql2 = """
                SELECT id, tier, content, tags, created_at, updated_at, user_id, FALSE
                FROM memories WHERE user_id = %s
            """
            params2: list[Any] = [uid]
            if tier:
                sql2 += " AND tier = %s"
                params2.append(tier.strip().lower())
            sql2 += " ORDER BY updated_at DESC LIMIT %s"
            params2.append(limit)
            with self._connect() as conn:
                rows = conn.execute(sql2, params2).fetchall()
        return [self._row_to_item(r) for r in rows]

    def recall(
        self,
        query: str,
        *,
        user_id: str | None = None,
        tier: str | None = None,
        top_k: int = 5,
    ) -> list[MemoryItem]:
        top_k = max(1, int(top_k or 5))
        uid = (user_id or "").strip() or "_anonymous"
        q = (query or "").strip()

        if self._vector_enabled and q:
            q_emb = embed_one(q)
            if q_emb is not None and len(q_emb) == self._embedding_dim:
                params: list[Any] = [vector_literal(q_emb), uid]
                sql = """
                    SELECT id, tier, content, tags, created_at, updated_at, user_id, TRUE
                    FROM memories
                    WHERE user_id = %s AND embedding IS NOT NULL
                """
                # fix param order: distance first in SELECT
                sql = """
                    SELECT id, tier, content, tags, created_at, updated_at, user_id, TRUE
                    FROM memories
                    WHERE user_id = %s AND embedding IS NOT NULL
                """
                params = [uid]
                if tier:
                    sql += " AND tier = %s"
                    params.append(tier.strip().lower())
                sql += " ORDER BY embedding <=> %s::vector ASC LIMIT %s"
                params.extend([vector_literal(q_emb), top_k])
                try:
                    with self._connect() as conn:
                        rows = conn.execute(sql, params).fetchall()
                    if rows:
                        return [self._row_to_item(r) for r in rows]
                except Exception as e:  # noqa: BLE001
                    logger.warning("vector recall failed, falling back to keywords: %s", e)

        candidates = self.list_items(user_id=user_id, tier=tier, limit=200)
        return _keyword_recall(candidates, q, top_k)

    @staticmethod
    def _row_to_item(row: Any) -> MemoryItem:
        tags = list(row[3] or [])
        created = row[4]
        updated = row[5]
        has_emb = bool(row[7]) if len(row) > 7 else False
        return MemoryItem(
            id=str(row[0]),
            tier=str(row[1]),
            content=str(row[2] or ""),
            tags=tags,
            created_at=created.isoformat() if hasattr(created, "isoformat") else str(created),
            updated_at=updated.isoformat() if hasattr(updated, "isoformat") else str(updated),
            user_id=str(row[6]) if row[6] else None,
            has_embedding=has_emb,
        )


class MemoryStore:
    """Facade used by runtime tools/API. Prefer Postgres when available."""

    def __init__(self, user_id: str | None = None, path: Path | None = None) -> None:
        self.user_id = (user_id or "").strip() or None
        self._backend: Any
        self._backend_kind = "json"
        dsn = database_url()
        try:
            self._backend = _PGMemoryBackend(dsn)
            self._backend_kind = "postgres"
        except Exception:  # noqa: BLE001
            self._backend = _JSONMemoryBackend(path or store_path_for_user(self.user_id))
            self._backend_kind = "json"

    @property
    def backend_kind(self) -> str:
        return self._backend_kind

    @property
    def vector_enabled(self) -> bool:
        return bool(getattr(self._backend, "vector_enabled", False))

    def write(
        self,
        content: str,
        tier: str = "note",
        tags: Iterable[str] | None = None,
    ) -> MemoryItem:
        return self._backend.write(content, tier=tier, tags=tags, user_id=self.user_id)

    def list(self, tier: str | None = None, limit: int = 50) -> list[MemoryItem]:
        return self._backend.list_items(user_id=self.user_id, tier=tier, limit=limit)

    def recall(
        self,
        query: str,
        *,
        tier: str | None = None,
        top_k: int = 5,
    ) -> list[MemoryItem]:
        return self._backend.recall(query, user_id=self.user_id, tier=tier, top_k=top_k)

    def to_public(self, item: MemoryItem) -> dict[str, Any]:
        d = asdict(item)
        d.pop("user_id", None)
        return d

    def maybe_import_json(self) -> int:
        if self._backend_kind != "postgres" or not self.user_id:
            return 0
        if self.list(limit=1):
            return 0
        path = store_path_for_user(self.user_id)
        if not path.is_file():
            return 0
        legacy = _JSONMemoryBackend(path)
        imported = 0
        for item in legacy.list_items(limit=200):
            try:
                self.write(item.content, tier=item.tier, tags=item.tags)
                imported += 1
            except Exception:  # noqa: BLE001
                continue
        return imported


_user_stores: dict[str, MemoryStore] = {}
_user_stores_lock = threading.Lock()


def get_store(user_id: str | None = None) -> MemoryStore:
    key = (user_id or "").strip() or "_global"
    with _user_stores_lock:
        store = _user_stores.get(key)
        if store is None:
            store = MemoryStore(user_id=user_id if key != "_global" else None)
            try:
                store.maybe_import_json()
            except Exception:  # noqa: BLE001
                pass
            _user_stores[key] = store
        return store


def clear_store_cache() -> None:
    with _user_stores_lock:
        _user_stores.clear()
