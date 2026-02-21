#!/bin/sh
set -eu

start_gui_stack() {
  export DISPLAY="${DISPLAY:-:99}"
  WIDTH="${NLM_BROWSER_WIDTH:-1366}"
  HEIGHT="${NLM_BROWSER_HEIGHT:-768}"
  DEPTH="${NLM_BROWSER_DEPTH:-24}"
  VNC_PORT="${NLM_VNC_PORT:-5900}"

  # Start X server only if not already available.
  if ! ps -ef 2>/dev/null | grep -E "Xvfb[[:space:]]+${DISPLAY}" | grep -v grep >/dev/null 2>&1; then
    Xvfb "${DISPLAY}" -screen 0 "${WIDTH}x${HEIGHT}x${DEPTH}" >/tmp/xvfb.log 2>&1 &
    sleep 1
  fi

  # Lightweight WM improves compatibility for headful browser.
  fluxbox >/tmp/fluxbox.log 2>&1 &

  # Expose desktop to VNC client.
  x11vnc -display "${DISPLAY}" -forever -shared -rfbport "${VNC_PORT}" -nopw -quiet >/tmp/x11vnc.log 2>&1 &

  if [ "${NLM_OPEN_BROWSER_ON_BOOT:-false}" = "true" ]; then
    BROWSER_BIN="$(command -v chromium-browser || command -v chromium || true)"
    if [ -n "${BROWSER_BIN}" ]; then
      "${BROWSER_BIN}" \
        --no-first-run \
        --no-default-browser-check \
        --disable-dev-shm-usage \
        --no-sandbox \
        --user-data-dir="${NLM_BROWSER_USER_DATA_DIR:-/root/.config/chromium}" \
        "https://notebooklm.google.com/" >/tmp/chromium-ui.log 2>&1 &
    fi
  fi
}

if [ "${NLM_ENABLE_VNC_BROWSER:-false}" = "true" ]; then
  start_gui_stack
fi

exec "$@"
