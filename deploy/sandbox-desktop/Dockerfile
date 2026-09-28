# open-bot sandbox desktop: Xvfb + fluxbox + x11vnc + noVNC
# Build: make sandbox-image-desktop
# Exposes noVNC on 6080 (proxied by Go API; do not publish to LAN).
#
# Alternative (heavier pull): set SANDBOX_DESKTOP_IMAGE=lscr.io/linuxserver/webtop:alpine-xfce
# and SANDBOX_DESKTOP_PORT=3000 in .env — see docs/沙箱电脑.md

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive \
    DISPLAY=:99 \
    LANG=C.UTF-8

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    curl \
    git \
    python3 \
    xvfb \
    fluxbox \
    x11vnc \
    novnc \
    websockify \
    fonts-dejavu-core \
    xterm \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 6080
LABEL openbot.sandbox.desktop="novnc"

ENTRYPOINT ["/entrypoint.sh"]
