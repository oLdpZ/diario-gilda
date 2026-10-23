-- Core/Parse.lua: lettura dei messaggi di sistema (bottino, creazione, duello) dalle GlobalStrings del client.
--   DG.Parse.pattern(fmt)    GlobalString -> pattern Lua ancorato; secondo valore: per ogni cattura, il numero
--                            dell'argomento (%1$s -> 1; per %s/%d l'ordine di apparizione)
--   DG.Parse.init(globals)   tabella con le GlobalStrings (quelle mancanti vengono ignorate)
--   DG.Parse.loot(msg)       { kind = "loot"|"craft", link, n } o nil
--   DG.Parse.duel(msg)       { winner, loser, how = "knockout"|"retreat" } o nil
-- Nessuna funzione solleva errori: input non stringa o non riconosciuto -> nil.
local _, DG = ...

local Parse = {}
DG.Parse = Parse

local function escape(s)
  return (s:gsub("[%^%$%(%)%%%.%[%]%*%+%-%?]", "%%%0"))
end

function Parse.pattern(fmt)
  local out, order = { "^" }, {}
  local i, len, seq = 1, #fmt, 0
  while i <= len do
    local c = fmt:sub(i, i)
    if c == "%" then
      local pos, conv = fmt:match("^%%(%d+)%$([sd])", i)
      local whole
      if pos then
        whole = #pos + 3
      else
        conv = fmt:match("^%%([sd])", i)
        whole = 2
      end
      if conv then
        seq = seq + 1
        order[#order + 1] = tonumber(pos) or seq
        out[#out + 1] = (conv == "s") and "(.+)" or "(%d+)"
        i = i + whole
      else
        -- "%%" o conversione sconosciuta: il carattere dopo il % è letterale
        out[#out + 1] = escape(fmt:sub(i + 1, i + 1))
        i = i + 2
      end
    else
      out[#out + 1] = escape(c)
      i = i + 1
    end
  end
  out[#out + 1] = "$"
  return table.concat(out), order
end

-- Liste ordinate: la variante "multipla" va provata prima (il suo suffisso "xN" altrimenti finirebbe nel link).
local LOOT_KEYS = {
  { "LOOT_ITEM_SELF_MULTIPLE", "loot", true },
  { "LOOT_ITEM_SELF", "loot", false },
  { "LOOT_ITEM_PUSHED_SELF_MULTIPLE", "loot", true },
  { "LOOT_ITEM_PUSHED_SELF", "loot", false },
  { "LOOT_ITEM_CREATED_SELF_MULTIPLE", "craft", true },
  { "LOOT_ITEM_CREATED_SELF", "craft", false },
}
local DUEL_KEYS = {
  { "DUEL_WINNER_KNOCKOUT", "knockout" },
  { "DUEL_WINNER_RETREAT", "retreat" },
}

local lootRules, duelRules = {}, {}

function Parse.init(globals)
  lootRules, duelRules = {}, {}
  if type(globals) ~= "table" then
    return
  end
  for _, e in ipairs(LOOT_KEYS) do
    local fmt = globals[e[1]]
    if type(fmt) == "string" then
      local pat = Parse.pattern(fmt)
      lootRules[#lootRules + 1] = { pat = pat, kind = e[2], multiple = e[3] }
    end
  end
  for _, e in ipairs(DUEL_KEYS) do
    local fmt = globals[e[1]]
    if type(fmt) == "string" then
      local pat, order = Parse.pattern(fmt)
      duelRules[#duelRules + 1] = { pat = pat, order = order, how = e[2] }
    end
  end
end

local function loot(msg)
  for _, r in ipairs(lootRules) do
    local link, n = msg:match(r.pat)
    if link then
      if r.multiple then
        return { kind = r.kind, link = link, n = tonumber(n) or 1 }
      end
      return { kind = r.kind, link = link, n = 1 }
    end
  end
  return nil
end

local function duel(msg)
  for _, r in ipairs(duelRules) do
    local caps = { msg:match(r.pat) }
    if #caps > 0 then
      local args = {}
      for idx, argNo in ipairs(r.order) do
        args[argNo] = caps[idx]
      end
      if args[1] and args[2] then
        return { winner = args[1], loser = args[2], how = r.how }
      end
    end
  end
  return nil
end

function Parse.loot(msg)
  if type(msg) ~= "string" then
    return nil
  end
  local ok, res = pcall(loot, msg)
  return ok and res or nil
end

function Parse.duel(msg)
  if type(msg) ~= "string" then
    return nil
  end
  local ok, res = pcall(duel, msg)
  return ok and res or nil
end
