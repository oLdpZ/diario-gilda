-- Core/Recorder.lua: trasforma gli eventi del gioco in eventi del diario (stato per istanza).
--   DG.Recorder.new(journal, deps) -> rec   journal = DG.Journal
--                                           deps = { now(), itemInfo(link) -> id, name, q, after(sec, fn) }
--   rec:onLootMsg(msg)                      messaggio CHAT_MSG_LOOT -> loot / craft
--   rec:onLootReady(isFishing, src, names)  LOOT_READY: sorgente del bottino; se pesca, nomi alle catture
--   rec:onLootClosed()                      LOOT_CLOSED: la sorgente vale ancora 2 s
--   rec:onFishCatch(name)                   aggiunge una cattura alla pesca in corso (la apre se serve)
--   rec:onSpellcast(spellId)                lanci di pesca: apre/estende lo span, chiuso dopo FISHING_IDLE
--   rec:onLevel(level) rec:onDead() rec:onBreath(active)
--   rec:onMoney(money, where)               calo >= SPEND_MIN -> spend
--   rec:onCombat(inCombat) rec:onHealth(cur, max)  minHp solo se c'e' stata almeno una lettura valida
--   rec:flush()                             chiude pesca e combattimento aperti (logout)
-- Ogni metodo pubblico e' avvolto in pcall: l'errore va in journal.logError, nessuna eccezione esce.
-- I valori passano da DG.Safe prima di essere letti. La parte 2 (Task 6) aggiunge metodi a DG.Recorder.
--   rec:onEncounterStart(id,name,diff,size) rec:onEncounterEnd(id,name,diff,size,success)  -> encounter
--   rec:onRoster(members) (debounce GROUP_SETTLE) -> group; rec:onQuestDone/onQuestAbandon -> quest
--   rec:onSystemMsg(msg) -> duel; rec:onZone(zone) -> zone
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

local closeFishing

-- Programma la chiusura per inattivita'; un timer piu' vecchio viene invalidato dal token.
local function armIdle(self)
  self.fishToken = self.fishToken + 1
  local token = self.fishToken
  self.deps.after(DG.Const.FISHING_IDLE, function()
    if self.fishing and self.fishToken == token then
      local ok, err = pcall(closeFishing, self)
      if not ok then
        pcall(self.journal.logError, "Recorder.fishIdle: " .. tostring(err))
      end
    end
  end)
end

local function openFishing(self)
  if not self.fishing then
    local t = self.deps.now()
    self.fishing = { from = t, last = t, casts = 0, catches = {} }
    armIdle(self)
    return self.fishing, true
  end
  return self.fishing, false
end

closeFishing = function(self)
  local f = self.fishing
  if not f then
    return
  end
  self.fishing = nil
  self.fishToken = self.fishToken + 1
  self.journal.add("fishing", { from = f.from, to = f.last, casts = f.casts, catches = f.catches })
end

-- Accoda una cattura allo span aperto (senza toccare il timer).
local function pushCatch(self, f, name)
  name = DG.Safe.str(name)
  if not name then
    return
  end
  if #f.catches < DG.Const.MAX_CATCHES then
    f.catches[#f.catches + 1] = name
  end
  f.last = self.deps.now()
end

-- Anche una cattura senza lancio riprogramma il timer: lo span non si spezza ogni FISHING_IDLE.
def("onFishCatch", function(self, name)
  if not DG.Safe.str(name) then
    return
  end
  local f, isNew = openFishing(self)
  pushCatch(self, f, name)
  if not isNew then
    armIdle(self)
  end
end)

def("onSpellcast", function(self, spellId)
  spellId = DG.Safe.num(spellId)
  if not spellId or not DG.Const.FISHING_SPELLS[spellId] then
    return
  end
  local f, isNew = openFishing(self)
  f.casts = f.casts + 1
  f.last = self.deps.now()
  if not isNew then
    armIdle(self)
  end
end)

-- Bottino -------------------------------------------------------------

local COLOR_Q = {
  ["9d9d9d"] = 0, ["ffffff"] = 1, ["1eff00"] = 2, ["0070dd"] = 3, ["a335ee"] = 4, ["ff8000"] = 5, ["e6cc80"] = 6,
}

-- Dati dal link: id, nome, qualita' (|cnIQ<q>: moderno o colore legacy |cffRRGGBB).
local function parseLink(link)
  local id = tonumber(link:match("|Hitem:(%d+)"))
  local name = link:match("%[(.-)%]")
  local q = tonumber(link:match("|cnIQ(%d+):"))
  if not q then
    local hex = link:match("|cff(%x%x%x%x%x%x)")
    q = hex and COLOR_Q[hex:lower()]
  end
  return id, name, q
end

def("onLootReady", function(self, isFishing, sourceName, itemNames)
  if isFishing then
    self.src = "fishing"
    local f, isNew = openFishing(self)
    if type(itemNames) == "table" then
      for _, name in ipairs(itemNames) do
        pushCatch(self, f, name)
      end
    end
    if not isNew then
      armIdle(self)
    end
  else
    self.src = DG.Safe.str(sourceName)
  end
  self.srcAt = self.src and self.deps.now() or nil
end)

-- Finestra di bottino chiusa: con l'autoloot veloce CHAT_MSG_LOOT puo' arrivare subito dopo, quindi la sorgente
-- resta ancora 2 s (la scadenza di SOURCE_TTL la scarta al massimo 2 s dopo), poi non vale piu'
-- (es. ricompense di missione).
def("onLootClosed", function(self)
  if self.src and self.srcAt then
    self.srcAt = math.min(self.srcAt, self.deps.now() - (SOURCE_TTL - 2))
  end
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
  if not name or not q then
    -- oggetto non ancora in cache: ricava i dati dal link
    local lid, lname, lq = parseLink(p.link)
    id, name, q = id or lid, name or lname, q or lq
  end
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
      -- minHp resta nil finche' non arriva una lettura valida della vita
      self.combat = { from = self.deps.now() }
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
  if self.combat.minHp == nil or pct < self.combat.minHp then
    self.combat.minHp = pct
  end
end)

def("flush", function(self)
  closeFishing(self)
  closeCombat(self)
end)

-- Boss ----------------------------------------------------------------

def("onEncounterStart", function(self, id, _name, _diff, _size)
  id = DG.Safe.num(id)
  if not id then
    return
  end
  self.encounters = self.encounters or {}
  self.encounters[id] = self.deps.now()
end)

def("onEncounterEnd", function(self, id, name, diff, size, success)
  id = DG.Safe.num(id)
  if not id then
    return
  end
  local t = self.deps.now()
  local from = self.encounters and self.encounters[id] or t
  if self.encounters then
    self.encounters[id] = nil
  end
  self.journal.add("encounter", {
    id = id, name = DG.Safe.str(name), diff = DG.Safe.num(diff), size = DG.Safe.num(size),
    ok = DG.Safe.num(success) == 1, from = from, to = t,
  })
end)

-- Gruppo --------------------------------------------------------------

local function nameKey(names)
  local copy = {}
  for i, n in ipairs(names) do
    copy[i] = n
  end
  table.sort(copy)
  return table.concat(copy, "\n")
end

def("onRoster", function(self, members)
  local names, guild = {}, {}
  for _, m in ipairs(type(members) == "table" and members or {}) do
    local name = type(m) == "table" and DG.Safe.str(m.name)
    if name then
      names[#names + 1] = name
      if DG.Safe.get(m.guild) == true then
        guild[#guild + 1] = name
      end
    end
  end
  self.rosterToken = (self.rosterToken or 0) + 1
  local token = self.rosterToken
  self.deps.after(DG.Const.GROUP_SETTLE, function()
    if self.rosterToken ~= token then
      return
    end
    local ok, err = pcall(function()
      local k = nameKey(names)
      if k ~= (self.lastRoster or "") then
        self.lastRoster = k
        self.journal.add("group", { members = names, guild = guild })
      end
    end)
    if not ok then
      pcall(self.journal.logError, "Recorder.rosterSettle: " .. tostring(err))
    end
  end)
end)

-- Missioni, duelli, zone ----------------------------------------------

def("onQuestDone", function(self, id, title, xp, money)
  self.journal.add("quest", {
    id = DG.Safe.num(id), title = DG.Safe.str(title), action = "done",
    xp = DG.Safe.num(xp), money = DG.Safe.num(money),
  })
end)

def("onQuestAbandon", function(self, id, title)
  self.journal.add("quest", { id = DG.Safe.num(id), title = DG.Safe.str(title), action = "abandon" })
end)

def("onSystemMsg", function(self, msg)
  msg = DG.Safe.str(msg)
  if not msg then
    return
  end
  local d = DG.Parse.duel(msg)
  if d then
    self.journal.add("duel", { winner = d.winner, loser = d.loser, how = d.how })
  end
end)

def("onZone", function(self, zone)
  zone = DG.Safe.str(zone)
  if not zone then
    return
  end
  self.journal.add("zone", { first = self.journal.firstVisit(zone) })
end)
