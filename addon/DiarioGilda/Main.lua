-- Main.lua: collegamento del Core al client WoW Forever. Unico file che tocca le API di gioco.
--   Un solo frame semplice (CreateFrame("Frame")) riceve gli eventi; nessun frame sicuro viene toccato.
--   Unico hook: hooksecurefunc("AbandonQuest", ...) (post-hook, non contamina).
--   ADDON_LOADED (il proprio) -> Safe.init, Parse.init, DiarioGildaDB = Journal.load, Recorder.new
--   PLAYER_ENTERING_WORLD solo con isLogin/isReload -> Journal.begin, zona, registro combattimenti
--   PLAYER_LOGOUT -> rec:flush(), Journal.finish(); gli altri eventi -> metodi di DG.Recorder
--   /diario: stato dell'addon; /diario debug: stampa ogni lancio del giocatore (spellId e nome)
-- Ogni handler e' avvolto in pcall: l'errore va in Journal.logError e non arriva mai al client.
local ADDON, DG = ...

local Safe, Journal = DG.Safe, DG.Journal
local PREFIX = "|cffffd100Diario di Gilda|r: "

local ready = false   -- vero dopo il nostro ADDON_LOADED
local rec = nil
local debugCasts = false
local shop = { merchant = false, auction = false, trainer = false }

local function say(text)
  print(PREFIX .. text)
end

-- Campo di una tabella di API (C_Map, C_Item, ...), nil se la tabella manca: Safe.call(nil) restituisce nil.
local function api(t, k)
  if type(t) == "table" then
    return t[k]
  end
  return nil
end

local function round3(n)
  return math.floor(n * 1000 + 0.5) / 1000
end

-- Posizione corrente per ogni evento del diario: { z, s, m, x, y, inst }.
local function where()
  local w = {
    z = Safe.str((Safe.call(GetZoneText))),
    s = Safe.str((Safe.call(GetSubZoneText))),
  }
  local m = Safe.num((Safe.call(api(C_Map, "GetBestMapForUnit"), "player")))
  w.m = m
  if Safe.get((Safe.call(IsInInstance))) == true then
    w.inst = Safe.str((Safe.call(GetInstanceInfo)))
  elseif m then
    local pos = Safe.get((Safe.call(api(C_Map, "GetPlayerMapPosition"), m, "player")))
    if type(pos) == "table" then
      local x, y = Safe.call(pos.GetXY, pos)
      x, y = Safe.num(x), Safe.num(y)
      if x and y then
        w.x, w.y = round3(x), round3(y)
      end
    end
  end
  return w
end

-- id, nome, qualita' di un oggetto dal link (nil se non ancora in cache).
local function itemInfo(link)
  local name, _, q = Safe.call(api(C_Item, "GetItemInfo"), link)
  local id = Safe.call(api(C_Item, "GetItemInfoInstant"), link)
  return id, name, q
end

local function after(sec, fn)
  C_Timer.After(sec, fn)
end

local function playerInfo()
  local _, class = Safe.call(UnitClass, "player")
  local _, race = Safe.call(UnitRace, "player")
  local version, buildNo = Safe.call(GetBuildInfo)
  version, buildNo = Safe.str(version), Safe.str(buildNo)
  local build = version
  if version and buildNo then
    build = version .. "." .. buildNo
  end
  return {
    char = Safe.str((Safe.call(UnitName, "player"))),
    realm = Safe.str((Safe.call(GetRealmName))),
    guild = Safe.str((Safe.call(GetGuildInfo, "player"))),
    class = Safe.str(class),
    race = Safe.str(race),
    level = Safe.num((Safe.call(UnitLevel, "player"))),
    build = build,
  }
end

local function combatLogOn()
  return Safe.get((Safe.call(LoggingCombat))) and true or false
end

local function questTitle(id)
  id = Safe.num(id)
  if not id then
    return nil
  end
  return Safe.call(api(C_QuestLog, "GetTitleForQuestID"), id)
end

local function spendWhere()
  if shop.merchant then
    return "merchant"
  elseif shop.auction then
    return "auction"
  elseif shop.trainer then
    return "trainer"
  end
  return "other"
end

-- Nome completo di un unit del gruppo ("Nome" o "Nome-Reame"); nil se secret o illeggibile.
local function memberName(unit)
  local name, realm = Safe.call(UnitName, unit)
  name = Safe.str(name)
  if not name then
    return nil
  end
  realm = Safe.str(realm)
  if realm then
    return name .. "-" .. realm
  end
  return name
end

-- Membri del gruppo, o nil se anche un solo nome non e' leggibile (si tiene l'ultimo gruppo noto).
local function roster()
  local prefix, count = "party", 4
  if Safe.get((Safe.call(IsInRaid))) == true then
    prefix, count = "raid", 40
  end
  local members = {}
  for i = 1, count do
    local unit = prefix .. i
    local exists = Safe.get((Safe.call(UnitExists, unit))) == true
    if exists and Safe.get((Safe.call(UnitIsUnit, unit, "player"))) ~= true then
      local name = memberName(unit)
      if not name then
        return nil
      end
      members[#members + 1] = { name = name, guild = Safe.get((Safe.call(UnitIsInMyGuild, unit))) == true }
    end
  end
  return members
end

-- Eventi ------------------------------------------------------------------

local handlers = {}

handlers.ADDON_LOADED = function(name)
  if name ~= ADDON then
    return
  end
  Safe.init(issecretvalue)
  DG.Parse.init(_G)
  DiarioGildaDB = Journal.load(DiarioGildaDB, { now = time, where = where })
  rec = DG.Recorder.new(Journal, { now = time, itemInfo = itemInfo, after = after })
  ready = true
end

handlers.PLAYER_ENTERING_WORLD = function(isLogin, isReload)
  -- anche i cambi di zona con schermata di caricamento passano di qui: quelli li gestisce ZONE_CHANGED_NEW_AREA
  if not (isLogin or isReload) then
    return
  end
  local info = playerInfo()
  if not info.char or not info.realm then
    Journal.logError("Main.PLAYER_ENTERING_WORLD: personaggio o reame non leggibili")
    return
  end
  Journal.begin(info, isReload and true or false)
  rec:onMoney(Safe.call(GetMoney))
  rec:onZone(Safe.call(GetZoneText))
  if not combatLogOn() then
    Safe.call(LoggingCombat, true)
  end
  if not combatLogOn() then
    say("il registro dei combattimenti è spento. Scrivi /combatlog per accenderlo.")
    Journal.add("note", { text = "registro dei combattimenti spento" })
  end
end

handlers.PLAYER_LOGOUT = function()
  rec:flush()
  Journal.finish()
end

handlers.ZONE_CHANGED_NEW_AREA = function()
  rec:onZone(Safe.call(GetZoneText))
end

handlers.CHAT_MSG_LOOT = function(msg)
  rec:onLootMsg(msg)
end

handlers.LOOT_READY = function()
  local isFishing = Safe.get((Safe.call(IsFishingLoot))) and true or false
  local source = nil
  if not isFishing and Safe.get((Safe.call(UnitIsDead, "target"))) == true then
    source = Safe.call(UnitName, "target")
  end
  local names = {}
  if isFishing then
    local n = Safe.num((Safe.call(GetNumLootItems))) or 0
    for i = 1, n do
      local _, itemName = Safe.call(GetLootSlotInfo, i)
      itemName = Safe.str(itemName)
      if itemName then
        names[#names + 1] = itemName
      end
    end
  end
  rec:onLootReady(isFishing, source, names)
end

handlers.UNIT_SPELLCAST_SUCCEEDED = function(unit, _, spellId)
  if Safe.get(unit) ~= "player" then
    return
  end
  rec:onSpellcast(spellId)
  if debugCasts then
    local id = Safe.num(spellId)
    local name = id and Safe.str((Safe.call(api(C_Spell, "GetSpellName"), id)))
    say("lancio " .. (id and tostring(id) or "?") .. " " .. (name or "?"))
  end
end

handlers.PLAYER_LEVEL_UP = function(level)
  rec:onLevel(level)
end

handlers.PLAYER_DEAD = function()
  rec:onDead()
end

handlers.MIRROR_TIMER_START = function(timer)
  if Safe.get(timer) == "BREATH" then
    rec:onBreath(true)
  end
end

handlers.MIRROR_TIMER_STOP = function(timer)
  if Safe.get(timer) == "BREATH" then
    rec:onBreath(false)
  end
end

handlers.PLAYER_MONEY = function()
  rec:onMoney(GetMoney(), spendWhere())
end

handlers.MERCHANT_SHOW = function() shop.merchant = true end
handlers.MERCHANT_CLOSED = function() shop.merchant = false end
handlers.AUCTION_HOUSE_SHOW = function() shop.auction = true end
handlers.AUCTION_HOUSE_CLOSED = function() shop.auction = false end
handlers.TRAINER_SHOW = function() shop.trainer = true end
handlers.TRAINER_CLOSED = function() shop.trainer = false end

handlers.PLAYER_REGEN_DISABLED = function()
  rec:onCombat(true)
end

handlers.PLAYER_REGEN_ENABLED = function()
  rec:onCombat(false)
end

handlers.UNIT_HEALTH = function(unit)
  if Safe.get(unit) ~= "player" then
    return
  end
  rec:onHealth(UnitHealth("player"), UnitHealthMax("player"))
end

handlers.ENCOUNTER_START = function(id, name, diff, size)
  rec:onEncounterStart(id, name, diff, size)
end

handlers.ENCOUNTER_END = function(id, name, diff, size, success)
  rec:onEncounterEnd(id, name, diff, size, success)
end

handlers.GROUP_ROSTER_UPDATE = function()
  local members = roster()
  if members then
    rec:onRoster(members)
  end
end

handlers.QUEST_TURNED_IN = function(id, xp, money)
  rec:onQuestDone(id, questTitle(id), xp, money)
end

handlers.CHAT_MSG_SYSTEM = function(msg)
  rec:onSystemMsg(msg)
end

-- Esegue un handler protetto; prima del nostro ADDON_LOADED c'e' solo ADDON_LOADED.
local function run(name, fn, ...)
  if not ready and name ~= "ADDON_LOADED" then
    return
  end
  local ok, err = pcall(fn, ...)
  if not ok then
    pcall(Journal.logError, "Main." .. name .. ": " .. tostring(err))
  end
end

local frame = CreateFrame("Frame")
for event in pairs(handlers) do
  frame:RegisterEvent(event)
end
frame:SetScript("OnEvent", function(_, event, ...)
  local fn = handlers[event]
  if fn then
    run(event, fn, ...)
  end
end)

-- Abbandono di una missione: post-hook, l'id arriva da C_QuestLog.GetAbandonQuest().
local function onAbandon()
  local id = Safe.call(api(C_QuestLog, "GetAbandonQuest"))
  rec:onQuestAbandon(id, questTitle(id))
end

if type(AbandonQuest) == "function" then
  pcall(hooksecurefunc, "AbandonQuest", function()
    run("AbandonQuest", onAbandon)
  end)
end

-- /diario -----------------------------------------------------------------

local function status()
  local s = Journal.current()
  local who = s and (s.char .. " (" .. s.realm .. ")") or "nessuno"
  say("versione " .. DG.Const.VERSION .. ", personaggio " .. who)
  say("sessione corrente: " .. (s and #s.events or 0) .. " eventi, registro dei combattimenti "
    .. (combatLogOn() and "acceso" or "spento"))
  say("sessioni salvate: " .. #DiarioGildaDB.sessions .. ", errori: " .. #DiarioGildaDB.errors)
end

local function slash(msg)
  msg = type(msg) == "string" and msg:lower():match("^%s*(.-)%s*$") or ""
  if msg == "debug" then
    debugCasts = not debugCasts
    say("debug dei lanci " .. (debugCasts and "acceso" or "spento"))
  else
    status()
  end
end

SLASH_DIARIO1 = "/diario"
SlashCmdList.DIARIO = function(msg)
  run("slash", slash, msg)
end
