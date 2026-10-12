# open-bot sandbox computer image (CLI)
# Tooling is installed into the image layer (/usr/local, /opt) so it survives
# container recreate — not under a user home that disappears with the image.
#
# Build:  make sandbox-image
# Verify: make sandbox-image-verify
# Tag:    openbot-sandbox:dev  (override with SANDBOX_IMAGE=...)
#
# Publish (optional, not done by make): tag + docker push to your registry, then
# set SANDBOX_IMAGE to that tag in .env / deploy. Local Docker Desktop does not
# publish a shared registry image unless you push.

FROM debian:bookworm-slim

ARG TARGETARCH
ARG NODE_VERSION=20.19.2
ARG PNPM_VERSION=10.33.4
ARG YARN_VERSION=1.22.22
ARG UV_VERSION=0.12.23
ARG BUN_VERSION=1.4.2

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    MAMBA_ROOT_PREFIX=/opt/micromamba \
    PATH=/usr/local/bin:/opt/micromamba/bin:/opt/micromamba/condabin:$PATH

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    curl \
    git \
    unzip \
    xz-utils \
    bzip2 \
    python3 \
    python3-pip \
    python3-venv \
    && rm -rf /var/lib/apt/lists/*

# --- Node.js LTS (official binary) + corepack pnpm/yarn ---
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) NARCH=x64 ;; \
      arm64) NARCH=arm64 ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NARCH}.tar.xz" \
      | tar -xJ -C /usr/local --strip-components=1; \
    corepack enable; \
    corepack prepare "pnpm@${PNPM_VERSION}" --activate; \
    corepack prepare "yarn@${YARN_VERSION}" --activate; \
    node -v; npm -v; pnpm -v; yarn -v

# --- uv (prefer for Python envs / tools) ---
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) UARCH=x86_64 ;; \
      arm64) UARCH=aarch64 ;; \
    esac; \
    curl -fsSL "https://github.com/astral-sh/uv/releases/download/${UV_VERSION}/uv-${UARCH}-unknown-linux-gnu.tar.gz" \
      | tar -xz -C /tmp; \
    install -m 0755 "/tmp/uv-${UARCH}-unknown-linux-gnu/uv" /usr/local/bin/uv; \
    install -m 0755 "/tmp/uv-${UARCH}-unknown-linux-gnu/uvx" /usr/local/bin/uvx; \
    rm -rf "/tmp/uv-${UARCH}-unknown-linux-gnu"; \
    uv --version

# --- bun ---
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) BARCH=x64 ;; \
      arm64) BARCH=aarch64 ;; \
    esac; \
    curl -fsSL "https://github.com/oven-sh/bun/releases/download/bun-v${BUN_VERSION}/bun-linux-${BARCH}.zip" -o /tmp/bun.zip; \
    unzip -q /tmp/bun.zip -d /tmp; \
    install -m 0755 "/tmp/bun-linux-${BARCH}/bun" /usr/local/bin/bun; \
    ln -sf /usr/local/bin/bun /usr/local/bin/bunx; \
    rm -rf /tmp/bun.zip "/tmp/bun-linux-${BARCH}"; \
    bun --version

# --- micromamba (conda-compatible) under /opt/micromamba ---
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) MARCH=64 ;; \
      arm64) MARCH=aarch64 ;; \
    esac; \
    curl -fsSL "https://micro.mamba.pm/api/micromamba/linux-${MARCH}/latest" -o /tmp/micromamba.tar.bz2; \
    mkdir -p /opt/micromamba /tmp/mm; \
    tar -xjf /tmp/micromamba.tar.bz2 -C /tmp/mm; \
    install -m 0755 /tmp/mm/bin/micromamba /usr/local/bin/micromamba; \
    ln -sf /usr/local/bin/micromamba /usr/local/bin/conda; \
    ln -sf /usr/local/bin/micromamba /usr/local/bin/mamba; \
    rm -rf /tmp/micromamba.tar.bz2 /tmp/mm; \
    micromamba shell init -s bash -r /opt/micromamba >/dev/null || true; \
    micromamba create -y -n py312 -c conda-forge python=3.12 pip; \
    micromamba clean -a -y; \
    micromamba --version; \
    micromamba run -n py312 python --version

# python → python3 shim (matches assistant-computer tooling expectations)
RUN printf '%s\n' '#!/bin/sh' 'exec python3 "$@"' > /usr/local/bin/python \
    && chmod +x /usr/local/bin/python

WORKDIR /workspace

# Phase 1: root inside container is OK for MVP; upgrade to non-root later.
CMD ["sleep", "infinity"]
