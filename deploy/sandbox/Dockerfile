# open-bot Phase-1 sandbox computer image (minimal CLI environment)
FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    curl \
    git \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Phase 1: root inside container is OK for MVP; upgrade to non-root later.
CMD ["sleep", "infinity"]
