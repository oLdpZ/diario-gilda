#!/usr/bin/env bash
# Crea dist/DiarioGilda-<versione>.zip con la cartella DiarioGilda/ (solo TOC, Core/ e Main.lua).
# La versione e' quella della riga "## Version:" del TOC. Funziona da Git Bash su Windows e su Linux:
# usa zip se c'e', altrimenti PowerShell Compress-Archive, altrimenti python -m zipfile.
set -e
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/addon/DiarioGilda"
TOC="$SRC/DiarioGilda.toc"

VERSION="$(sed -n 's/^## Version:[[:space:]]*//p' "$TOC" | tr -d '\r' | head -n 1)"
if [ -z "$VERSION" ]; then
  echo "Versione non trovata in $TOC" >&2
  exit 1
fi

OUT="$ROOT/dist/DiarioGilda-$VERSION.zip"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

# Copia solo cio' che serve al client, dentro la cartella DiarioGilda/.
mkdir -p "$STAGE/DiarioGilda" "$ROOT/dist"
cp "$TOC" "$SRC/Main.lua" "$STAGE/DiarioGilda/"
cp -r "$SRC/Core" "$STAGE/DiarioGilda/Core"
rm -f "$OUT"

if command -v zip >/dev/null 2>&1; then
  (cd "$STAGE" && zip -qr "$OUT" DiarioGilda)
elif PY="$(for c in python python3; do "$c" -I -c 'import zipfile' >/dev/null 2>&1 && { echo "$c"; break; }; done)"; [ -n "$PY" ]; then
  if command -v cygpath >/dev/null 2>&1; then
    STAGE_ARG="$(cygpath -m "$STAGE")"; OUT_ARG="$(cygpath -m "$OUT")"
  else
    STAGE_ARG="$STAGE"; OUT_ARG="$OUT"
  fi
  # Percorsi con "/" nello zip (Compress-Archive di Windows PowerShell 5 usa "\", che il client non gradisce).
  "$PY" -I - "$STAGE_ARG" "$OUT_ARG" <<'PYEOF'
import os, sys, zipfile
stage, out = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
    for base, _, files in os.walk(os.path.join(stage, "DiarioGilda")):
        for name in sorted(files):
            full = os.path.join(base, name)
            z.write(full, os.path.relpath(full, stage).replace(os.sep, "/"))
PYEOF
elif command -v powershell.exe >/dev/null 2>&1; then
  powershell.exe -NoProfile -Command "Compress-Archive -Path '$(cygpath -w "$STAGE/DiarioGilda")' -DestinationPath '$(cygpath -w "$OUT")'"
else
  echo "Serve zip, python o PowerShell per creare lo zip" >&2
  exit 1
fi

echo "Creato $OUT"
