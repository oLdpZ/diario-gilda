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

-- Finto client per Main.lua. Imposta i globali di gioco (stub delle API) e restituisce G:
--   G.v        valori letti dagli stub, modificabili dal test (unità, zona, soldi, bottino, ...)
--   G.secret   set dei valori che issecretvalue considera secret
--   G.printed  righe passate a print
--   G.timers   callback di C_Timer.After ({ sec, fn })
--   G.frames   frame creati con CreateFrame (eventi registrati e script)
-- H.ungame() rimuove tutto (chiamato anche all'inizio di ogni gameStub).
local gameNames = {}
local realPrint = print

function H.ungame()
  for name in pairs(gameNames) do
    _G[name] = nil
  end
  gameNames = {}
  _G.print = realPrint
  _G.DiarioGildaDB = nil
  _G.SLASH_DIARIO1 = nil
  H.game = nil
end

function H.gameStub()
  H.ungame()
  local G = { secret = {}, printed = {}, timers = {}, frames = {}, hooks = {} }
  local v = {
    now = 1793000000,
    units = {
      player = { name = "Gino", class = { "Guerriero", "WARRIOR" }, race = { "Umano", "Human" }, level = 12,
        hp = 100, hpMax = 100 },
    },
    raid = false,
    realm = "Realm", guild = "La Gilda",
    build = { "1.60.1", "70338", "Oct 1 2026", 16001 },
    zone = "Westfall", subzone = "Longshore", mapId = 1436, pos = { x = 0.51234, y = 0.30149 },
    inInstance = false, instanceName = nil,
    combatLog = false, combatLogCanEnable = true,
    items = {}, spells = {}, quests = {}, abandonId = nil,
    fishing = false, loot = {}, money = 0,
  }
  G.v = v

  local function set(name, value)
    gameNames[name] = true
    _G[name] = value
  end
  local function unit(u)
    return v.units[u]
  end

  set("CreateFrame", function()
    local f = { events = {}, scripts = {} }
    function f:RegisterEvent(e) self.events[e] = true end
    function f:UnregisterEvent(e) self.events[e] = nil end
    function f:SetScript(n, fn) self.scripts[n] = fn end
    G.frames[#G.frames + 1] = f
    return f
  end)
  set("issecretvalue", function(x) return G.secret[x] == true end)
  set("time", function() return v.now end)
  set("hooksecurefunc", function(name, fn)
    local orig = _G[name]
    G.hooks[#G.hooks + 1] = name
    _G[name] = function(...)
      orig(...)
      fn(...)
    end
  end)
  set("AbandonQuest", function() end)

  set("UnitName", function(u) local x = unit(u) if x then return x.name, x.realm end end)
  set("UnitExists", function(u) return unit(u) ~= nil end)
  set("UnitIsUnit", function(a, b) return a == b end)
  set("UnitIsDead", function(u) local x = unit(u) return x and x.dead or false end)
  set("UnitIsInMyGuild", function(u) local x = unit(u) return x and x.guild or false end)
  set("UnitClass", function(u) local x = unit(u) if x and x.class then return x.class[1], x.class[2] end end)
  set("UnitRace", function(u) local x = unit(u) if x and x.race then return x.race[1], x.race[2] end end)
  set("UnitLevel", function(u) local x = unit(u) return x and x.level end)
  set("UnitHealth", function(u) local x = unit(u) return x and x.hp end)
  set("UnitHealthMax", function(u) local x = unit(u) return x and x.hpMax end)
  set("IsInRaid", function() return v.raid end)
  set("GetRealmName", function() return v.realm end)
  set("GetGuildInfo", function() return v.guild end)
  set("GetBuildInfo", function() return unpack(v.build) end)

  set("GetZoneText", function() return v.zone end)
  set("GetSubZoneText", function() return v.subzone end)
  set("IsInInstance", function() return v.inInstance, v.inInstance and "party" or "none" end)
  set("GetInstanceInfo", function() return v.instanceName, "party", 1, "Normale" end)
  set("C_Map", {
    GetBestMapForUnit = function() return v.mapId end,
    GetPlayerMapPosition = function()
      if not v.pos then return nil end
      return { GetXY = function() return v.pos.x, v.pos.y end }
    end,
  })

  set("LoggingCombat", function(on)
    if on ~= nil and v.combatLogCanEnable then v.combatLog = on end
    return v.combatLog
  end)
  set("C_Item", {
    GetItemInfo = function(link) local it = v.items[link] if it then return it.name, link, it.q end end,
    GetItemInfoInstant = function(link) local it = v.items[link] return it and it.id end,
  })
  set("C_Timer", { After = function(sec, fn) G.timers[#G.timers + 1] = { sec = sec, fn = fn } end })
  set("C_Spell", { GetSpellName = function(id) return v.spells[id] end })
  set("C_QuestLog", {
    GetTitleForQuestID = function(id) return v.quests[id] end,
    GetAbandonQuest = function() return v.abandonId end,
  })
  set("IsFishingLoot", function() return v.fishing end)
  set("GetNumLootItems", function() return #v.loot end)
  set("GetLootSlotInfo", function(i) return 134400, v.loot[i], 1, nil, 3 end)
  set("GetMoney", function() return v.money end)

  set("LOOT_ITEM_SELF", "You receive loot: %s.")
  set("LOOT_ITEM_SELF_MULTIPLE", "You receive loot: %sx%d.")
  set("DUEL_WINNER_KNOCKOUT", "%1$s has defeated %2$s in a duel")
  set("SlashCmdList", {})
  _G.print = function(...)
    local parts = {}
    for i = 1, select("#", ...) do parts[i] = tostring((select(i, ...))) end
    G.printed[#G.printed + 1] = table.concat(parts, " ")
  end

  H.game = G
  return G
end

-- Consegna un evento a ogni frame che lo ha registrato, come fa il client.
function H.fire(event, ...)
  for _, f in ipairs(H.game.frames) do
    if f.events[event] and f.scripts.OnEvent then
      f.scripts.OnEvent(f, event, ...)
    end
  end
end

-- Esegue un comando slash registrato (es. H.slash("DIARIO", "debug")).
function H.slash(key, msg)
  _G.SlashCmdList[key](msg or "")
end

return H
