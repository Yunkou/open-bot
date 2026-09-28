#!/bin/bash
# Lightweight desktop stack: Xvfb + fluxbox + x11vnc + noVNC (port 6080).
set -euo pipefail

DISPLAY_NUM="${DISPLAY_NUM:-99}"
export DISPLAY=":${DISPLAY_NUM}"
GEOMETRY="${VNC_GEOMETRY:-1280x800x24}"
VNC_PORT="${VNC_PORT:-5900}"
NOVNC_PORT="${NOVNC_PORT:-6080}"
NOVNC_WEB="${NOVNC_WEB:-/usr/share/novnc}"

mkdir -p /tmp/.X11-unix /var/log
chmod 1777 /tmp/.X11-unix || true

echo "[openbot-desktop] starting Xvfb on ${DISPLAY} (${GEOMETRY})"
Xvfb "${DISPLAY}" -screen 0 "${GEOMETRY}" -ac +extension GLX +render -noreset \
  > /var/log/xvfb.log 2>&1 &
sleep 0.8

echo "[openbot-desktop] starting fluxbox"
fluxbox > /var/log/fluxbox.log 2>&1 &
sleep 0.3

echo "[openbot-desktop] starting x11vnc on ${VNC_PORT}"
x11vnc -display "${DISPLAY}" -forever -shared -rfbport "${VNC_PORT}" \
  -nopw -listen 127.0.0.1 -xkb -ncache 10 -ncache_cr \
  > /var/log/x11vnc.log 2>&1 &
sleep 0.5

# Prefer index that autoconnects when present.
if [[ -f "${NOVNC_WEB}/vnc.html" && ! -e "${NOVNC_WEB}/index.html" ]]; then
  ln -sf vnc.html "${NOVNC_WEB}/index.html" || true
fi

echo "[openbot-desktop] starting websockify/noVNC on ${NOVNC_PORT}"
exec websockify --web="${NOVNC_WEB}" "${NOVNC_PORT}" "127.0.0.1:${VNC_PORT}"
