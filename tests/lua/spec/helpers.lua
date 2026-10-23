-- Helper dei test: carica i file dell'addon come fa il client (vararg = nome addon, namespace).
local H = {}
local ROOT = (debug.getinfo(1, "S").source:match("^@(.*)/tests/lua/spec/helpers%.lua$")) or "."
H.ROOT = ROOT
H.ADDON_DIR = ROOT .. "/addon/DiarioGilda"

-- Carica i file (percorsi relativi a addon/DiarioGilda/) in ordine, in un unico namespace; lo restituisce.
-- Gli ambienti sono quelli standard di Lua: i globali di gioco non esistono finché un test non li definisce.
function H.load(files, ns)
  ns = ns or {}
  for _, rel in ipairs(files) do
    local chunk, err = loadfile(H.ADDON_DIR .. "/" .. rel)
    assert(chunk, err)
    chunk("DiarioGilda", ns)
  end
  return ns
end

function H.readFile(path)
  local f = assert(io.open(path, "rb"))
  local s = f:read("*a")
  f:close()
  return s
end

return H
