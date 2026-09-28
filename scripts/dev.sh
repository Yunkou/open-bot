#!/usr/bin/env bash
# Optional helper: print how to start the three Phase 1 processes.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cat <<EOF
open-bot Phase 1 — start each in its own terminal from repo root:

  make dev-runtime   # http://127.0.0.1:8001
  make dev-api       # http://127.0.0.1:18080
  make dev-web       # http://127.0.0.1:5173

Optional infra:
  make compose-up

Copy env:
  cp .env.example .env   # then fill OPENAI_API_KEY if you want a real model

Repo: $ROOT
EOF
