"""Batch backfill memories.embedding for rows missing vectors.

Usage (from services/agent-runtime with venv + DATABASE_URL / EMBEDDING_*):

  python -m app.backfill_embeddings
  python -m app.backfill_embeddings --batch-size 32 --limit 500
  python -m app.backfill_embeddings --dry-run

Idempotent: only updates rows WHERE embedding IS NULL.
Optional --also-mem0 is a no-op placeholder (Mem0 OSS manages its own vectors).
"""

from __future__ import annotations

import argparse
import logging
import os
import sys
from typing import Any

from .embedding import embed_texts, embedding_config, vector_literal

logger = logging.getLogger("open-bot.backfill")


def _connect():
    url = (os.getenv("DATABASE_URL") or "").strip()
    if not url:
        raise SystemExit("DATABASE_URL required")
    import psycopg

    return psycopg.connect(url)


def count_missing(conn) -> int:
    with conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM memories WHERE embedding IS NULL")
        row = cur.fetchone()
        return int(row[0] if row else 0)


def fetch_batch(conn, batch_size: int) -> list[tuple[str, str]]:
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT id, content FROM memories
            WHERE embedding IS NULL
            ORDER BY created_at ASC NULLS FIRST, id ASC
            LIMIT %s
            """,
            (batch_size,),
        )
        rows = cur.fetchall() or []
    out: list[tuple[str, str]] = []
    for rid, content in rows:
        text = (content or "").strip()
        if text:
            out.append((str(rid), text))
    return out


def update_embeddings(conn, pairs: list[tuple[str, list[float]]]) -> int:
    if not pairs:
        return 0
    n = 0
    with conn.cursor() as cur:
        for mid, vec in pairs:
            lit = vector_literal(vec)
            cur.execute(
                "UPDATE memories SET embedding = %s::vector WHERE id = %s AND embedding IS NULL",
                (lit, mid),
            )
            n += cur.rowcount
    conn.commit()
    return n


def run(*, batch_size: int, limit: int | None, dry_run: bool) -> dict[str, Any]:
    base, _key, model, dim = embedding_config()
    conn = _connect()
    missing = count_missing(conn)
    updated = 0
    scanned = 0
    failed_batches = 0
    print(f"missing={missing} embedding={base} model={model} dim={dim} dry_run={dry_run}")
    try:
        while True:
            if limit is not None and scanned >= limit:
                break
            take = batch_size
            if limit is not None:
                take = min(batch_size, limit - scanned)
            batch = fetch_batch(conn, take)
            if not batch:
                break
            scanned += len(batch)
            texts = [t for _, t in batch]
            if dry_run:
                print(f"dry-run batch size={len(batch)} first_id={batch[0][0]}")
                continue
            vectors = embed_texts(texts, timeout=60.0)
            if not vectors or len(vectors) != len(batch):
                failed_batches += 1
                logger.warning("embed batch failed size=%s", len(batch))
                # skip these ids by writing nothing; avoid infinite loop by advancing via temp mark?
                # Mark with zero-vector is bad. Break to avoid hot loop if service down.
                break
            # dim check
            good: list[tuple[str, list[float]]] = []
            for (mid, _), vec in zip(batch, vectors):
                if len(vec) != dim:
                    logger.warning("dim mismatch id=%s got=%s want=%s", mid, len(vec), dim)
                    continue
                good.append((mid, vec))
            updated += update_embeddings(conn, good)
            print(f"progress scanned={scanned} updated={updated}")
            if len(batch) < take:
                break
    finally:
        conn.close()
    return {
        "missing_before": missing,
        "scanned": scanned,
        "updated": updated,
        "failed_batches": failed_batches,
        "dry_run": dry_run,
    }


def main(argv: list[str] | None = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    ap = argparse.ArgumentParser(description="Backfill memories.embedding")
    ap.add_argument("--batch-size", type=int, default=int(os.getenv("EMBEDDING_BACKFILL_BATCH") or "32"))
    ap.add_argument("--limit", type=int, default=None)
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--also-mem0", action="store_true", help="No-op: Mem0 OSS owns its vectors")
    args = ap.parse_args(argv)
    if args.also_mem0:
        print("note: --also-mem0 ignored (Mem0 collection managed separately)")
    result = run(batch_size=max(1, args.batch_size), limit=args.limit, dry_run=args.dry_run)
    print(result)
    return 0 if result.get("failed_batches", 0) == 0 else 2


if __name__ == "__main__":
    raise SystemExit(main())
