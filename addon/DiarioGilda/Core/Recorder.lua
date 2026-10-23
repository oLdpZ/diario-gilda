-- Core/Recorder.lua: trasforma gli eventi del gioco in eventi del diario (stato per istanza).
--   DG.Recorder.new(journal, deps) -> rec   journal = DG.Journal
--                                           deps = { now(), itemInfo(link) -> id, name, q, after(sec, fn) }
--   rec:onLootMsg(msg)                      messaggio CHAT_MSG_LOOT -> loot / craft
--   rec:onLootReady(isFishing, src, names)  LOOT_READY: sorgente del bottino; se pesca, nomi alle catture
--   rec:onFishCatch(name)                   aggiunge una cattura alla pesca in corso (la apre se serve)
--   rec:onSpellcast(spellId)                lanci di pesca: apre/estende lo span, chiuso dopo FISHING_IDLE
--   rec:onLevel(level) rec:onDead() rec:onBreath(active)
--   rec:onMoney(money, where)               calo >= SPEND_MIN -> spend
--   rec:onCombat(inCombat) rec:onHealth(cur, max)
--   rec:flush()                             chiude pesca e combattimento aperti (logout)
-- Ogni metodo pubblico e' avvolto in pcall: l'errore va in journal.logError, nessuna eccezione esce.
-- I valori passano da DG.Safe prima di essere letti. La parte 2 (Task 6) aggiunge metodi a DG.Recorder.
local _, DG = ...

local Recorder = {}
Recorder.__index = Recorder
DG.Recorder = Recorder

local SOURCE_TTL = 10

-- Definisce un metodo pubblico protetto da pcall.
local function def(name, fn)
  Recorder[name] = function(self, ...)
    local ok, err = pcall(fn, self, ...)
    if not ok then
      pcall(self.journal.logError, "Recorder." .. name .. ": " .. tostring(err))
    end
  end
end
Recorder.def = def

function Recorder.new(journal, deps)
  return setmetatable({
    journal = journal, deps = deps,
    src = nil, srcAt = nil,
    fishing = nil, fishToken = 0,
    combat = nil,
    breath = false, money = nil,
  }, Recorder)
end

-- Pesca ---------------------------------------------------------------

local function openFishing(self)
  if not self.fishing then
    local t = self.deps.now()
    self.fishing = { from = t, last = t, casts = 0, catches = {} }
  end
  return self.fishing
end

local function closeFishing(self)
  local f = self.fishing
  if not f then
    return
  end
  self.fishing = nil
  self.fishToken = self.fishToken + 1
  self.journal.add("fishing", { from = f.from, to = f.last, casts = f.casts, catches = f.catches })
end

local function addCatch(self, name)
  name = DG.Safe.str(name)
  if not name then
    return
  end
  local f = openFishing(self)
  if #f.catches < DG.Const.MAX_CATCHES then
    f.catches[#f.catches + 1] = name
  end
  f.last = self.deps.now()
end

def("onFishCatch", addCatch)

def("onSpellcast", function(self, spellId)
  spellId = DG.Safe.num(spellId)
  if not spellId or not DG.Const.FISHING_SPELLS[spellId] then
    return
  end
  local f = openFishing(self)
  f.casts = f.casts + 1
  f.last = self.deps.now()
  self.fishToken = self.fishToken + 1
  local token = self.fishToken
  self.deps.after(DG.Const.FISHING_IDLE, function()
    -- un lancio piu' recente ha invalidato questo timer
    if self.fishing and self.fishToken == token then
      local ok, err = pcall(closeFishing, self)
      if not ok then
        pcall(self.journal.logError, "Recorder.fishIdle: " .. tostring(err))
      end
    end
  end)
end)

-- Bottino -------------------------------------------------------------

def("onLootReady", function(self, isFishing, sourceName, itemNames)
  if isFishing then
    self.src = "fishing"
    openFishing(self)
    if type(itemNames) == "table" then
      for _, name in ipairs(itemNames) do
        addCatch(self, name)
      end
    end
  else
    self.src = DG.Safe.str(sourceName)
  end
  self.srcAt = self.src and self.deps.now() or nil
end)

def("onLootMsg", function(self, msg)
  msg = DG.Safe.str(msg)
  if not msg then
    return
  end
  local p = DG.Parse.loot(msg)
  if not p then
    return
  end
  local id, name, q = self.deps.itemInfo(p.link)
  id, name, q = DG.Safe.num(id), DG.Safe.str(name), DG.Safe.num(q)
  if not name or not q or q < DG.Const.MIN_QUALITY then
    return
  end
  local fields = { item = name, id = id, q = q, n = p.n }
  if p.kind == "loot" and self.src and self.srcAt and self.deps.now() - self.srcAt <= SOURCE_TTL then
    fields.src = self.src
  end
  self.journal.add(p.kind, fields)
end)

-- Livello, morte, soldi -----------------------------------------------

def("onLevel", function(self, level)
  level = DG.Safe.num(level)
  if level then
    self.journal.add("level", { level = level })
  end
end)

def("onBreath", function(self, active)
  self.breath = DG.Safe.get(active) and true or false
end)

def("onDead", function(self)
  self.journal.add("death", { breath = self.breath })
end)

def("onMoney", function(self, money, where)
  money = DG.Safe.num(money)
  if not money then
    return
  end
  local prev = self.money
  self.money = money
  if prev and prev - money >= DG.Const.SPEND_MIN then
    self.journal.add("spend", { amount = prev - money, where = DG.Safe.str(where) or "other" })
  end
end)

-- Combattimento -------------------------------------------------------

local function closeCombat(self)
  local c = self.combat
  if not c then
    return
  end
  self.combat = nil
  self.journal.add("combat", { from = c.from, to = self.deps.now(), minHp = c.minHp })
end

def("onCombat", function(self, inCombat)
  if DG.Safe.get(inCombat) then
    closeFishing(self)
    if not self.combat then
      self.combat = { from = self.deps.now(), minHp = 100 }
    end
  else
    closeCombat(self)
  end
end)

def("onHealth", function(self, cur, max)
  cur, max = DG.Safe.num(cur), DG.Safe.num(max)
  if not self.combat or not cur or not max or max <= 0 then
    return
  end
  local pct = math.floor(100 * cur / max)
  if pct < self.combat.minHp then
    self.combat.minHp = pct
  end
end)

def("flush", function(self)
  closeFishing(self)
  closeCombat(self)
end)
