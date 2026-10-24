-- Genera (e verifica) tests/fixtures/DiarioGilda.lua guidando il vero Recorder/Journal con eventi simulati.
-- Se il file manca lo scrive e fallisce chiedendo di rieseguire. Orologio finto: il contenuto e' stabile.
local H = require("helpers")

local FILES = {
  "Core/Const.lua", "Core/Safe.lua", "Core/Journal.lua", "Core/Parse.lua", "Core/Recorder.lua", "Main.lua",
}
local RARE = "|cnIQ3:|Hitem:1234::::::::|h[Spada Rara]|h|r"
local PATH = H.ROOT .. "/tests/fixtures/DiarioGilda.lua"
local START = 1793000000

local G, v

local function login()
  H.fire("ADDON_LOADED", "DiarioGilda")
  H.fire("PLAYER_ENTERING_WORLD", true, false)
end

local function runTimers()
  local list = G.timers
  G.timers = {}
  for _, t in ipairs(list) do t.fn() end
end

local function advance(sec)
  v.now = v.now + sec
end

-- Sessione 1: serata completa, chiusa con il logout.
local function playFirstSession()
  v.guild = "I Murloc Rampanti"
  v.now = START
  login()

  -- pesca: 3 lanci e una cattura, poi subito un combattimento che la chiude
  v.fishing = true
  v.loot = { "Pesce Gatto" }
  for _ = 1, 3 do
    advance(15)
    H.fire("UNIT_SPELLCAST_SUCCEEDED", "player", "guid", 7620)
  end
  advance(5)
  H.fire("LOOT_READY")
  H.fire("LOOT_CLOSED")
  v.fishing = false
  v.loot = {}
  advance(10)
  H.fire("PLAYER_REGEN_DISABLED")
  v.units.player.hp = 12
  H.fire("UNIT_HEALTH", "player")
  advance(20)
  v.units.player.hp = 100
  H.fire("PLAYER_REGEN_ENABLED")

  -- bottino raro da un mob
  advance(30)
  v.items[RARE] = { name = "Spada Rara", q = 3, id = 1234 }
  v.units.target = { name = "Defias Bandito", dead = true }
  H.fire("LOOT_READY")
  H.fire("CHAT_MSG_LOOT", "You receive loot: " .. RARE .. ".")
  H.fire("LOOT_CLOSED")
  v.units.target = nil

  advance(60)
  v.units.player.level = 13
  H.fire("PLAYER_LEVEL_UP", 13)

  -- missione consegnata
  advance(45)
  v.quests[42] = "Il Ragno"
  H.fire("QUEST_TURNED_IN", 42, 450, 1200)

  -- gruppo di 5 con due membri di gilda, poi ingresso in istanza e boss
  advance(120)
  v.units.party1 = { name = "Bob", guild = true }
  v.units.party2 = { name = "Ann", guild = true }
  v.units.party3 = { name = "Carl" }
  v.units.party4 = { name = "Dora", realm = "Altro" }
  H.fire("GROUP_ROSTER_UPDATE")
  advance(5)
  runTimers()

  advance(60)
  v.zone, v.subzone, v.mapId, v.pos = "Deadmines", "", 291, nil
  v.inInstance, v.instanceName = true, "Deadmines"
  H.fire("ZONE_CHANGED_NEW_AREA")
  advance(300)
  H.fire("ENCOUNTER_START", 1144, "Edwin VanCleef", 1, 5)
  advance(95)
  H.fire("ENCOUNTER_END", 1144, "Edwin VanCleef", 1, 5, 1)

  advance(600)
  H.fire("PLAYER_LOGOUT")
end

-- Sessione 2: la sera dopo, il gioco va in crash (nessun PLAYER_LOGOUT, quindi nessun stop).
local function playCrashedSession()
  v.now = START + 86400
  v.zone, v.subzone, v.mapId, v.pos = "Westfall", "Sentinel Hill", 1436, { x = 0.5, y = 0.5 }
  v.inInstance, v.instanceName = false, nil
  v.units.party1, v.units.party2, v.units.party3, v.units.party4 = nil, nil, nil, nil
  login()
  advance(240)
  v.units.player.level = 14
  H.fire("PLAYER_LEVEL_UP", 14)
  advance(90)
  v.units.player.hp = 40
  H.fire("PLAYER_REGEN_DISABLED")
  H.fire("UNIT_HEALTH", "player")
  advance(15)
  H.fire("PLAYER_REGEN_ENABLED")
end

describe("fixture SavedVariables", function()
  before_each(function()
    G = H.gameStub()
    v = G.v
    v.combatLog = true
    -- versione fissa: la fixture non deve cambiare a ogni release dell'addon
    H.load(FILES).Const.VERSION = "0.1.0"
  end)

  after_each(function()
    H.ungame()
  end)

  it("tests/fixtures/DiarioGilda.lua corrisponde a cio' che l'addon registra", function()
    playFirstSession()
    local saved = DiarioGildaDB
    -- secondo avvio del client: stato nuovo, SavedVariables ricaricate dal disco
    G = H.gameStub()
    v = G.v
    v.combatLog = true
    v.guild = "I Murloc Rampanti"
    v.units.player.level = 13
    H.load(FILES).Const.VERSION = "0.1.0"
    _G.DiarioGildaDB = saved
    playCrashedSession()
    assert.equals(2, #DiarioGildaDB.sessions)
    assert.is_truthy(DiarioGildaDB.sessions[1].stop)
    assert.is_nil(DiarioGildaDB.sessions[2].stop)
    assert.equals(0, #DiarioGildaDB.errors)

    local text = H.serialize("DiarioGildaDB", DiarioGildaDB)
    local f = io.open(PATH, "rb")
    if not f then
      local w = assert(io.open(PATH, "wb"))
      w:write(text)
      w:close()
      error("fixture creata in " .. PATH .. ": rieseguire i test")
    end
    local expected = f:read("*a"):gsub("\r\n", "\n")
    f:close()
    assert.equals(expected, text)

    -- il file e' Lua valido e ricostruisce la stessa tabella
    local chunk = assert(loadstring(expected, "fixture"))
    local env = {}
    setfenv(chunk, env)
    chunk()
    assert.same(DiarioGildaDB, env.DiarioGildaDB)
  end)
end)
