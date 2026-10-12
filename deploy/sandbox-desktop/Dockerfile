# open-bot sandbox desktop: Xvfb + fluxbox + x11vnc + noVNC
# Extends the CLI sandbox image so node/python/conda tooling stays in the image.
# Build: make sandbox-image-desktop  (builds CLI base first)
# Exposes noVNC on 6080 (proxied by Go API; do not publish to LAN).
#
# Alternative (heavier pull): set SANDBOX_DESKTOP_IMAGE=lscr.io/linuxserver/webtop:alpine-xfce
# and SANDBOX_DESKTOP_PORT=3000 in .env — see docs/沙箱电脑.md

ARG SANDBOX_BASE=openbot-sandbox:dev
FROM ${SANDBOX_BASE}

ENV DEBIAN_FRONTEND=noninteractive \
    DISPLAY=:99 \
    LANG=C.UTF-8

RUN set -eux; \
    ok=0; \
    for i in 1 2 3 4 5; do \
      if apt-get update && apt-get install -y --no-install-recommends \
        xvfb fluxbox x11vnc novnc websockify fonts-dejavu-core xterm; then \
        ok=1; break; \
      fi; \
      echo "apt retry $i failed; sleeping"; sleep $((i * 20)); \
    done; \
    test "$ok" = 1; \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 6080
LABEL openbot.sandbox.desktop="novnc"

ENTRYPOINT ["/entrypoint.sh"]
