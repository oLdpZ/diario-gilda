local H = require("helpers")

local FILES = {
  "Core/Const.lua", "Core/Safe.lua", "Core/Journal.lua", "Core/Parse.lua", "Core/Recorder.lua", "Main.lua",
}
local RARE = "|cnIQ3:|Hitem:1234::::::::|h[Spada Rara]|h|r"
local COMBATLOG_OFF = "|cffffd100Diario di Gilda|r: il registro dei combattimenti è spento. "
  .. "Scrivi /combatlog per accenderlo."

describe("Main (collegamento al gioco)", function()
  local G, v

  local function session() return DiarioGildaDB.sessions[#DiarioGildaDB.sessions] end
  local function events() return session().events end
  local function kinds()
    local out = {}
    for _, e in ipairs(events()) do out[#out + 1] = e.k end
    return out
  end
  local function find(k)
    for _, e in ipairs(events()) do
      if e.k == k then return e end
    end
  end
  local function printedHas(text)
    for _, line in ipairs(G.printed) do
      if line:find(text, 1, true) then return true end
    end
    return false
  end
  local function login()
    H.fire("ADDON_LOADED", "DiarioGilda")
    H.fire("PLAYER_ENTERING_WORLD", true, false)
  end
  -- Nuovo client (stub e Main ricaricati); setup(G) prepara gli stub prima del caricamento di Main.
  local function restart(setup)
    G = H.gameStub()
    v = G.v
    v.combatLog = true
    if setup then setup(G) end
    H.load(FILES)
  end
  local function count(k)
    local n = 0
    for _, e in ipairs(events()) do
      if e.k == k then n = n + 1 end
    end
    return n
  end
  local function runTimers()
    local list = G.timers
    G.timers = {}
    for _, t in ipairs(list) do t.fn() end
  end

  before_each(function()
    G = H.gameStub()
    v = G.v
    v.combatLog = true
    H.load(FILES)
  end)

  after_each(function()
    H.ungame()
  end)

  it("ADDON_LOADED + PLAYER_ENTERING_WORLD(login): sessione del personaggio e un evento zone", function()
    login()
    local s = session()
    assert.equals(1, #DiarioGildaDB.sessions)
    assert.equals("Gino", s.char)
    assert.equals("Realm", s.realm)
    assert.equals("La Gilda", s.guild)
    assert.equals("WARRIOR", s.class)
    assert.equals("Human", s.race)
    assert.equals(12, s.level)
    assert.equals("1.60.1.70338", s.build)
    local z = find("zone")
    assert.truthy(z)
    assert.is_true(z.first)
    assert.equals("Westfall", z.z)
    assert.equals("Longshore", z.s)
    assert.equals(1436, z.m)
    assert.equals(0.512, z.x)
    assert.equals(0.301, z.y)
    assert.equals(0, #DiarioGildaDB.errors)
  end)

  it("in istanza: inst valorizzato, niente x/y", function()
    v.inInstance = true
    v.instanceName = "Deadmines"
    v.pos = nil
    login()
    local z = find("zone")
    assert.equals("Deadmines", z.inst)
    assert.is_nil(z.x)
    assert.is_nil(z.y)
  end)

  it("PLAYER_ENTERING_WORLD da cambio zona non apre una nuova sessione", function()
    login()
    H.fire("PLAYER_ENTERING_WORLD", false, false)
    assert.equals(1, #DiarioGildaDB.sessions)
    assert.equals(1, #kinds())
  end)

  it("ZONE_CHANGED_NEW_AREA: evento zone", function()
    login()
    v.zone = "Duskwood"
    H.fire("ZONE_CHANGED_NEW_AREA")
    assert.equals("Duskwood", events()[#events()].z)
    assert.equals("zone", events()[#events()].k)
  end)

  it("PLAYER_LOGOUT: stop valorizzato", function()
    login()
    v.now = v.now + 3600
    H.fire("PLAYER_LOGOUT")
    assert.equals(v.now, session().stop)
  end)

  it("CHAT_MSG_LOOT con oggetto raro: evento loot (con la fonte da LOOT_READY)", function()
    login()
    v.items[RARE] = { name = "Spada Rara", q = 3, id = 1234 }
    v.units.target = { name = "Defias", dead = true }
    H.fire("LOOT_READY")
    H.fire("CHAT_MSG_LOOT", "You receive loot: " .. RARE .. ".")
    local e = find("loot")
    assert.truthy(e)
    assert.equals("Spada Rara", e.item)
    assert.equals(1234, e.id)
    assert.equals(3, e.q)
    assert.equals(1, e.n)
    assert.equals("Defias", e.src)
  end)

  it("LOOT_READY in pesca: i nomi degli oggetti diventano catture", function()
    login()
    v.fishing = true
    v.loot = { "Pesce Gatto", "Scatola Rara" }
    H.fire("LOOT_READY")
    H.fire("PLAYER_LOGOUT")
    local f = find("fishing")
    assert.same({ "Pesce Gatto", "Scatola Rara" }, f.catches)
  end)

  it("registro combattimenti spento: lo accende", function()
    v.combatLog = false
    login()
    assert.is_true(v.combatLog)
    assert.is_nil(find("note"))
    assert.is_false(printedHas("combatlog"))
  end)

  it("registro combattimenti che resta spento: messaggio stampato ed evento note", function()
    v.combatLog = false
    v.combatLogCanEnable = false
    login()
    assert.truthy(find("note"))
    assert.truthy(find("note").text)
    assert.is_true(printedHas(COMBATLOG_OFF))
  end)

  it("un'API che lancia errore dentro un handler: nessuna eccezione, errors ha una voce", function()
    login()
    _G.UnitHealth = function() error("boom") end
    assert.has_no.errors(function() H.fire("UNIT_HEALTH", "player") end)
    assert.equals(1, #DiarioGildaDB.errors)
    assert.truthy(DiarioGildaDB.errors[1]:find("UNIT_HEALTH", 1, true))
  end)

  it("sessione salvata malformata: Journal.begin solleva, Main lo intercetta", function()
    _G.DiarioGildaDB = { v = 1, sessions = { "rotta" } }
    assert.has_no.errors(function() login() end)
    assert.equals(1, #DiarioGildaDB.errors)
  end)

  it("eventi prima di ADDON_LOADED non sollevano errori", function()
    assert.has_no.errors(function()
      H.fire("PLAYER_ENTERING_WORLD", true, false)
      H.fire("CHAT_MSG_LOOT", "x")
      H.slash("DIARIO", "")
    end)
  end)

  it("/diario stampa una riga con gli eventi", function()
    login()
    H.slash("DIARIO", "")
    assert.is_true(printedHas("eventi"))
    assert.is_true(printedHas("Gino"))
    assert.is_true(printedHas(H.load({ "Core/Const.lua" }).Const.VERSION))
  end)

  it("/diario debug stampa i lanci del giocatore", function()
    login()
    v.spells[7620] = "Pesca"
    H.slash("DIARIO", "debug")
    H.fire("UNIT_SPELLCAST_SUCCEEDED", "player", "guid", 7620)
    assert.is_true(printedHas("7620"))
    assert.is_true(printedHas("Pesca"))
    H.slash("DIARIO", "debug")
    G.printed = {}
    H.fire("UNIT_SPELLCAST_SUCCEEDED", "player", "guid", 7620)
    assert.equals(0, #G.printed)
  end)

  it("UNIT_SPELLCAST_SUCCEEDED di un altro unit viene ignorato", function()
    login()
    H.fire("UNIT_SPELLCAST_SUCCEEDED", "party1", "guid", 7620)
    H.fire("PLAYER_LOGOUT")
    assert.is_nil(find("fishing"))
  end)

  it("spesa dal mercante: where = merchant", function()
    v.money = 1000000
    login()
    H.fire("MERCHANT_SHOW")
    v.money = 900000
    H.fire("PLAYER_MONEY")
    H.fire("MERCHANT_CLOSED")
    v.money = 800000
    H.fire("PLAYER_MONEY")
    local list = {}
    for _, e in ipairs(events()) do
      if e.k == "spend" then list[#list + 1] = e end
    end
    assert.equals(2, #list)
    assert.equals("merchant", list[1].where)
    assert.equals(100000, list[1].amount)
    assert.equals("other", list[2].where)
  end)

  it("combattimento con vita minima e morte con respiro", function()
    login()
    H.fire("PLAYER_REGEN_DISABLED")
    v.units.player.hp = 25
    H.fire("UNIT_HEALTH", "player")
    H.fire("PLAYER_REGEN_ENABLED")
    assert.equals(25, find("combat").minHp)
    H.fire("MIRROR_TIMER_START", "BREATH", 60, 60, -1, false, "Respiro")
    H.fire("PLAYER_DEAD")
    assert.is_true(find("death").breath)
  end)

  it("PLAYER_LEVEL_UP ed ENCOUNTER_START/END", function()
    login()
    H.fire("PLAYER_LEVEL_UP", 13)
    assert.equals(13, find("level").level)
    H.fire("ENCOUNTER_START", 1234, "Edwin", 1, 5)
    v.now = v.now + 60
    H.fire("ENCOUNTER_END", 1234, "Edwin", 1, 5, 1)
    local e = find("encounter")
    assert.is_true(e.ok)
    assert.equals(60, e.to - e.from)
  end)

  it("GROUP_ROSTER_UPDATE: membri del gruppo e quelli in gilda", function()
    login()
    v.units.party1 = { name = "Bob", guild = true }
    v.units.party2 = { name = "Ann", realm = "Altro" }
    H.fire("GROUP_ROSTER_UPDATE")
    runTimers()
    local e = find("group")
    assert.same({ "Bob", "Ann-Altro" }, e.members)
    assert.same({ "Bob" }, e.guild)
  end)

  it("GROUP_ROSTER_UPDATE in incursione: raid1..40 senza il giocatore", function()
    login()
    v.raid = true
    v.units.raid1 = { name = "Gino" }
    v.units.raid2 = { name = "Bob" }
    _G.UnitIsUnit = function(a, b) return a == b or (a == "raid1" and b == "player") end
    H.fire("GROUP_ROSTER_UPDATE")
    runTimers()
    assert.same({ "Bob" }, find("group").members)
  end)

  it("GROUP_ROSTER_UPDATE con un nome secret: tiene l'ultimo gruppo noto", function()
    login()
    v.units.party1 = { name = "Bob" }
    H.fire("GROUP_ROSTER_UPDATE")
    runTimers()
    v.units.party2 = { name = "NASCOSTO" }
    G.secret["NASCOSTO"] = true
    H.fire("GROUP_ROSTER_UPDATE")
    assert.equals(0, #G.timers)
    runTimers()
    local n = 0
    for _, e in ipairs(events()) do
      if e.k == "group" then n = n + 1 end
    end
    assert.equals(1, n)
    assert.equals(0, #DiarioGildaDB.errors)
  end)

  it("QUEST_TURNED_IN", function()
    login()
    v.quests[42] = "Il Ragno"
    H.fire("QUEST_TURNED_IN", 42, 450, 1200)
    local q = find("quest")
    assert.equals("Il Ragno", q.title)
    assert.equals("done", q.action)
    assert.equals(450, q.xp)
    assert.equals(1200, q.money)
  end)

  it("abbandono via C_QuestLog: id ricordato a SetAbandonQuest, un solo evento", function()
    login()
    v.quests[7] = "Lupi"
    v.abandonId = 7
    C_QuestLog.SetAbandonQuest()
    C_QuestLog.AbandonQuest() -- azzera l'id prima del post-hook
    local a = events()[#events()]
    assert.equals("quest", a.k)
    assert.equals("abandon", a.action)
    assert.equals(7, a.id)
    assert.equals("Lupi", a.title)
    assert.equals(1, count("quest"))
    assert.same({ "C_QuestLog.SetAbandonQuest", "C_QuestLog.AbandonQuest", "AbandonQuest" }, G.hooks)
    -- un secondo abbandono piu' tardi viene registrato
    v.now = v.now + 30
    v.quests[8] = "Orsi"
    v.abandonId = 8
    C_QuestLog.SetAbandonQuest()
    C_QuestLog.AbandonQuest()
    assert.equals(2, count("quest"))
    assert.equals(8, events()[#events()].id)
    assert.equals(0, #DiarioGildaDB.errors)
  end)

  it("abbandono con solo l'AbandonQuest globale (fallback)", function()
    restart(function()
      rawset(C_QuestLog, "SetAbandonQuest", nil)
      rawset(C_QuestLog, "AbandonQuest", nil)
    end)
    login()
    v.quests[7] = "Lupi"
    v.abandonId = 7
    AbandonQuest()
    local a = events()[#events()]
    assert.equals("abandon", a.action)
    assert.equals(7, a.id)
    assert.equals("Lupi", a.title)
    assert.same({ "AbandonQuest" }, G.hooks)
  end)

  it("AbandonQuest globale che chiama C_QuestLog.AbandonQuest: nessun doppio evento", function()
    restart(function()
      _G.AbandonQuest = function() C_QuestLog.AbandonQuest() end
    end)
    login()
    v.quests[7] = "Lupi"
    v.abandonId = 7
    C_QuestLog.SetAbandonQuest()
    AbandonQuest()
    assert.equals(1, count("quest"))
    assert.equals(7, events()[#events()].id)
  end)

  it("senza AbandonQuest globale (client moderno) si aggancia solo C_QuestLog", function()
    restart(function() _G.AbandonQuest = nil end)
    login()
    assert.same({ "C_QuestLog.SetAbandonQuest", "C_QuestLog.AbandonQuest" }, G.hooks)
  end)

  it("CHAT_MSG_SYSTEM: duello", function()
    login()
    H.fire("CHAT_MSG_SYSTEM", "Gino has defeated Bob in a duel")
    local d = find("duel")
    assert.equals("Gino", d.winner)
    assert.equals("knockout", d.how)
  end)

  it("un evento che il client non conosce: gli altri funzionano, errors lo nomina", function()
    restart(function(g) g.badEvents.TRAINER_SHOW = true end)
    login()
    H.fire("PLAYER_LEVEL_UP", 13)
    assert.truthy(find("zone"))
    assert.equals(13, find("level").level)
    assert.equals(1, #DiarioGildaDB.errors)
    assert.truthy(DiarioGildaDB.errors[1]:find("TRAINER_SHOW", 1, true))
  end)

  it("UNIT_HEALTH e UNIT_SPELLCAST_SUCCEEDED registrati solo per player", function()
    local f = G.frames[1]
    assert.same({ "player" }, f.units.UNIT_HEALTH)
    assert.same({ "player" }, f.units.UNIT_SPELLCAST_SUCCEEDED)
  end)

  it("LOOT_READY ripetuto nella stessa finestra conta una volta sola", function()
    login()
    v.fishing = true
    v.loot = { "Pesce Gatto" }
    H.fire("LOOT_READY")
    H.fire("LOOT_READY")
    H.fire("LOOT_CLOSED")
    H.fire("LOOT_READY")
    H.fire("LOOT_CLOSED")
    H.fire("PLAYER_LOGOUT")
    assert.same({ "Pesce Gatto", "Pesce Gatto" }, find("fishing").catches)
  end)

  it("LOOT_CLOSED perso: dopo 30 s un nuovo LOOT_READY viene gestito", function()
    login()
    v.fishing = true
    v.loot = { "Pesce Gatto" }
    H.fire("LOOT_READY")
    v.now = v.now + 31
    v.loot = { "Luccio" }
    H.fire("LOOT_READY")
    H.fire("PLAYER_LOGOUT")
    assert.same({ "Pesce Gatto", "Luccio" }, find("fishing").catches)
  end)

  it("LOOT_CLOSED: una ricompensa poco dopo non prende il mob come fonte", function()
    login()
    v.items[RARE] = { name = "Spada Rara", q = 3, id = 1234 }
    v.units.target = { name = "Defias", dead = true }
    H.fire("LOOT_READY")
    H.fire("LOOT_CLOSED")
    v.now = v.now + 3
    H.fire("CHAT_MSG_LOOT", "You receive loot: " .. RARE .. ".")
    assert.is_nil(find("loot").src)
  end)

  it("GROUP_ROSTER_UPDATE con un membro \"Unknown\": tiene l'ultimo gruppo noto", function()
    login()
    v.units.party1 = { name = "Bob" }
    H.fire("GROUP_ROSTER_UPDATE")
    runTimers()
    v.units.party2 = { name = "Unknown" }
    H.fire("GROUP_ROSTER_UPDATE")
    assert.equals(0, #G.timers)
    assert.equals(1, count("group"))
    assert.same({ "Bob" }, find("group").members)
  end)

  it("ingresso in combattimento gia' ferito senza altri danni: minHp dalla vita corrente", function()
    login()
    v.units.player.hp = 40
    H.fire("PLAYER_REGEN_DISABLED")
    H.fire("PLAYER_REGEN_ENABLED")
    assert.equals(40, find("combat").minHp)
  end)

  it("gilda non leggibile al login: PLAYER_GUILD_UPDATE la scrive nella sessione", function()
    v.guild = nil
    login()
    assert.is_nil(session().guild)
    v.guild = "La Gilda"
    H.fire("PLAYER_GUILD_UPDATE", "player")
    assert.equals("La Gilda", session().guild)
  end)

  it("/reload entro 600 s dopo PLAYER_LOGOUT: stessa sessione, stesso id, eventi accodati", function()
    login()
    H.fire("PLAYER_LEVEL_UP", 13)
    v.now = v.now + 100
    H.fire("PLAYER_LOGOUT")
    local saved, id, n = DiarioGildaDB, session().id, #events()
    assert.truthy(session().stop)
    -- il client ricarica l'interfaccia: Lua da capo, SavedVariables dal disco
    local now = v.now
    restart()
    v.now = now + 20
    _G.DiarioGildaDB = saved
    H.fire("ADDON_LOADED", "DiarioGilda")
    H.fire("PLAYER_ENTERING_WORLD", false, true)
    H.fire("PLAYER_LEVEL_UP", 14)
    assert.equals(1, #DiarioGildaDB.sessions)
    assert.equals(id, session().id)
    assert.is_nil(session().stop)
    assert.is_true(#events() > n)
    assert.equals(14, events()[#events()].level)
    assert.equals(0, #DiarioGildaDB.errors)
  end)

  it("un solo frame e /diario registrato", function()
    assert.equals(1, #G.frames)
    assert.equals("/diario", SLASH_DIARIO1)
  end)
end)
