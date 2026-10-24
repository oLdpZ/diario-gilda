#!/usr/bin/env bash
# Build locale Windows del programmino, con gli stessi flag del workflow di release.
# Crea dist/DiarioGilda-windows-amd64.zip con DiarioGilda.exe e VERSION (la versione e'
# la riga "## Version:" del TOC dell'addon) e, se FIRMA_KEY e' impostata, anche il .sig.
#
#   read -rs DIARIO_WEBHOOK && export DIARIO_WEBHOOK     # incolla l'URL, Invio (non si vede)
#   read -rs FIRMA_KEY && export FIRMA_KEY               # facoltativo, per il .sig
#   DIARIO_PUBKEY=<PUBLIC_KEY> bash scripts/build-app.sh
#
# I segreti non vanno sulla riga di comando: finirebbero nella cronologia della shell.
# Se DIARIO_WEBHOOK non e' impostata e lo script gira in un terminale, lo chiede
# (senza mostrarlo); Invio a vuoto = build senza webhook.
# Senza DIARIO_WEBHOOK l'app non invia nulla; senza DIARIO_PUBKEY non si aggiorna da sola.
# Il webhook non viene mai stampato. Funziona da Git Bash su Windows e su Linux:
# usa zip se c'e', altrimenti python -m zipfile, altrimenti PowerShell Compress-Archive.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOC="$ROOT/addon/DiarioGilda/DiarioGilda.toc"
PKG="github.com/oLdpZ/diario-gilda/app/internal/build"
ASSET="DiarioGilda-windows-amd64.zip"

if ! command -v go >/dev/null 2>&1 && [ -x "/c/Program Files/Go/bin/go.exe" ]; then
  export PATH="/c/Program Files/Go/bin:$PATH"
fi

VERSION="$(sed -n 's/^## Version:[[:space:]]*//p' "$TOC" | tr -d '\r' | head -n 1)"
if [ -z "$VERSION" ]; then
  echo "Versione non trovata in $TOC" >&2
  exit 1
fi

WEBHOOK="${DIARIO_WEBHOOK:-}"
if [ -z "$WEBHOOK" ] && [ -t 0 ]; then
  read -rsp "URL del webhook (non viene mostrato; Invio a vuoto = nessuno): " WEBHOOK
  echo >&2
fi
PUBKEY="${DIARIO_PUBKEY:-}"
[ -n "$WEBHOOK" ] || echo "ATTENZIONE: DIARIO_WEBHOOK vuota, questa build non inviera' nulla a Discord" >&2
[ -n "$PUBKEY" ] || echo "ATTENZIONE: DIARIO_PUBKEY vuota, questa build non si aggiornera' da sola" >&2

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
mkdir -p "$ROOT/dist"
OUT="$ROOT/dist/$ASSET"
rm -f "$OUT" "$OUT.sig"

echo "Build DiarioGilda $VERSION (windows/amd64)"
(
  cd "$ROOT"
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath \
    -ldflags "-H windowsgui -s -w -X $PKG.Version=$VERSION -X $PKG.WebhookURL=$WEBHOOK -X $PKG.PublicKey=$PUBKEY" \
    -o "$STAGE/DiarioGilda.exe" ./app/cmd/diario
)
printf '%s\n' "$VERSION" > "$STAGE/VERSION"

if command -v zip >/dev/null 2>&1; then
  (cd "$STAGE" && zip -q "$OUT" DiarioGilda.exe VERSION)
elif PY="$(for c in python python3; do "$c" -I -c 'import zipfile' >/dev/null 2>&1 && { echo "$c"; break; }; done)"; [ -n "$PY" ]; then
  if command -v cygpath >/dev/null 2>&1; then
    STAGE_ARG="$(cygpath -m "$STAGE")"; OUT_ARG="$(cygpath -m "$OUT")"
  else
    STAGE_ARG="$STAGE"; OUT_ARG="$OUT"
  fi
  "$PY" -I - "$STAGE_ARG" "$OUT_ARG" <<'PYEOF'
import os, sys, zipfile
stage, out = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
    for name in ("DiarioGilda.exe", "VERSION"):
        z.write(os.path.join(stage, name), name)
PYEOF
elif command -v powershell.exe >/dev/null 2>&1; then
  powershell.exe -NoProfile -Command "Compress-Archive -Path '$(cygpath -w "$STAGE/DiarioGilda.exe")','$(cygpath -w "$STAGE/VERSION")' -DestinationPath '$(cygpath -w "$OUT")'"
else
  echo "Serve zip, python o PowerShell per creare lo zip" >&2
  exit 1
fi
echo "Creato $OUT"

if [ -n "${FIRMA_KEY:-}" ]; then
  (cd "$ROOT" && go run ./app/cmd/firma sign "$OUT")
  echo "Creato $OUT.sig"
else
  echo "FIRMA_KEY non impostata: zip non firmato (firma con: go run ./app/cmd/firma sign dist/$ASSET)"
fi
