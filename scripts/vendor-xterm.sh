#!/bin/sh
# Pin xterm.js 5.5.0 + FitAddon into web/vendor/xterm (no CDN at runtime).
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
DEST="$ROOT/web/vendor/xterm"
mkdir -p "$DEST"

XTERM_JS="https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/lib/xterm.js"
XTERM_CSS="https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/css/xterm.css"
FIT_JS="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.8.0/lib/xterm-addon-fit.js"
UNICODE11_JS="https://cdn.jsdelivr.net/npm/@xterm/addon-unicode11@0.9.0/lib/addon-unicode11.js"

curl -fsSL "$XTERM_JS" -o "$DEST/xterm.js"
curl -fsSL "$XTERM_CSS" -o "$DEST/xterm.css"
curl -fsSL "$FIT_JS" -o "$DEST/addon-fit.js"
curl -fsSL "$UNICODE11_JS" -o "$DEST/addon-unicode11.js"

echo "vendored xterm.js 5.5.0 + addon-unicode11 0.9.0 into $DEST"
