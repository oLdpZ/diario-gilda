#!/usr/bin/env bash
# Esegue busted e luacheck. Da Git Bash su Windows rilancia se stesso in WSL (distro archlinux);
# su Linux (CI) esegue direttamente. Il percorso del progetto contiene spazi: tutto tra virgolette.
set -e
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ "$(uname -s)" != "Linux" ]; then
  WINROOT="$(cygpath -m "$ROOT")"
  MSYS_NO_PATHCONV=1 exec wsl --cd "$WINROOT" -d archlinux -- bash scripts/test-lua.sh "$@"
fi

cd "$ROOT"
# Attiva luarocks (Lua 5.1) se installato nell'home; su CI busted/luacheck sono già nel PATH.
if command -v luarocks >/dev/null 2>&1 && [ -d "$HOME/.luarocks" ]; then
  eval "$(luarocks --lua-version=5.1 --tree="$HOME/.luarocks" path)"
  export PATH="$HOME/.luarocks/bin:$PATH"
fi
busted
luacheck addon tests
