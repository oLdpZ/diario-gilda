-- Core/Safe.lua: scarto dei valori "secret" (il client li nasconde in dungeon/incontri).
--   DG.Safe.init(isSecret)  funzione che dice se un valore è secret (nil: nulla è secret, es. nei test)
--   DG.Safe.get(v)          v, o nil se secret (il valore secret non viene mai letto né confrontato)
--   DG.Safe.str(v)          stringa non vuota e non secret, altrimenti nil
--   DG.Safe.num(v)          numero non secret, altrimenti nil
--   DG.Safe.call(fn, ...)   pcall: i valori di ritorno, o nil se fn va in errore
local _, DG = ...

local Safe = {}
DG.Safe = Safe

local isSecret = nil

function Safe.init(fn)
  isSecret = fn
end

function Safe.get(v)
  if isSecret and isSecret(v) then
    return nil
  end
  return v
end

function Safe.str(v)
  v = Safe.get(v)
  if type(v) == "string" and v ~= "" then
    return v
  end
  return nil
end

function Safe.num(v)
  v = Safe.get(v)
  if type(v) == "number" then
    return v
  end
  return nil
end

function Safe.call(fn, ...)
  local res = { pcall(fn, ...) }
  if not res[1] then
    return nil
  end
  return unpack(res, 2, #res)
end
