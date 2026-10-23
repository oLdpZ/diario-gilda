local H = require("helpers")

local GLOBALS = {
  LOOT_ITEM_SELF = "You receive loot: %s.",
  LOOT_ITEM_SELF_MULTIPLE = "You receive loot: %sx%d.",
}
local RARE = "|cff0070dd|Hitem:2042::::::::|h[Staff of Westfall]|h|r"
local GREEN = "|cff1eff00|Hitem:2043::::::::|h[Green Thing]|h|r"

describe("Recorder (bottino, pesca, livelli, morti, soldi, combattimento)", function()
  local DG, J, db, rec, clock, timers, items, secret

  local function events() return J.current().events end
  local function kinds()
    local out = {}
    for _, e in ipairs(events()) do out[#out + 1] = e.k end
    return out
  end

  before_each(function()
    clock = 1000
    timers = {}
    secret = {}
    items = {
      [RARE] = { 2042, "Staff of Westfall", 3 },
      [GREEN] = { 2043, "Green Thing", 2 },
    }
    DG = H.load({ "Core/Const.lua", "Core/Safe.lua", "Core/Journal.lua", "Core/Parse.lua", "Core/Recorder.lua" })
    DG.Safe.init(function(v) return secret[v] == true end)
    DG.Parse.init(GLOBALS)
    J = DG.Journal
    db = J.load(nil, { now = function() return clock end, where = function() return { z = "Westfall" } end })
    J.begin({ char = "Gino", realm = "R", class = "WARRIOR", race = "Human", level = 1, build = "1" }, false)
    rec = DG.Recorder.new(J, {
      now = function() return clock end,
      itemInfo = function(link)
        local i = items[link]
        if i == "boom" then error("boom") end
        if i then return i[1], i[2], i[3] end
      end,
      after = function(sec, fn) timers[#timers + 1] = { sec = sec, fn = fn } end,
    })
  end)

  it("loot raro con sorgente recente", function()
    rec:onLootReady(false, "Murloc Forager")
    clock = 1003
    rec:onLootMsg("You receive loot: " .. RARE .. ".")
    local e = events()[1]
    assert.equals("loot", e.k)
    assert.equals("Staff of Westfall", e.item)
    assert.equals(2042, e.id)
    assert.equals(3, e.q)
    assert.equals(1, e.n)
    assert.equals("Murloc Forager", e.src)
  end)

  it("la sorgente scade dopo 10 secondi", function()
    rec:onLootReady(false, "Murloc Forager")
    clock = 1011
    rec:onLootMsg("You receive loot: " .. RARE .. ".")
    assert.is_nil(events()[1].src)
  end)

  it("loot verde: nessun evento", function()
    rec:onLootMsg("You receive loot: " .. GREEN .. ".")
    assert.equals(0, #events())
  end)

  it("messaggio secret: nessun evento e nessun errore", function()
    local msg = "You receive loot: " .. RARE .. "."
    secret[msg] = true
    rec:onLootMsg(msg)
    assert.equals(0, #events())
    assert.equals(0, #db.errors)
  end)

  it("itemInfo in errore: nessun evento, un errore registrato", function()
    items[RARE] = "boom"
    rec:onLootMsg("You receive loot: " .. RARE .. ".")
    assert.equals(0, #events())
    assert.equals(1, #db.errors)
    assert.is_truthy(db.errors[1]:find("boom", 1, true))
  end)

  it("itemInfo nil: qualita, nome e id dal link", function()
    items[RARE] = nil
    rec:onLootMsg("You receive loot: " .. RARE .. ".")
    local e = events()[1]
    assert.equals("loot", e.k)
    assert.equals(3, e.q)
    assert.equals("Staff of Westfall", e.item)
    assert.equals(2042, e.id)
  end)

  it("itemInfo nil: link verde scartato, formato |cnIQ4: accettato", function()
    items[GREEN] = nil
    rec:onLootMsg("You receive loot: " .. GREEN .. ".")
    assert.equals(0, #events())
    local epic = "|cnIQ4:|Hitem:77::::::::|h[Epic Thing]|h|r"
    rec:onLootMsg("You receive loot: " .. epic .. ".")
    assert.equals(4, events()[1].q)
    assert.equals(77, events()[1].id)
  end)

  it("pesca: loot senza lancio programma il timer di chiusura", function()
    rec:onLootReady(true, nil, { "Fish" })
    assert.equals(1, #timers)
    clock = 1200
    timers[1].fn()
    assert.same({ "fishing" }, kinds())
  end)

  it("pesca: due lanci e una cattura, un solo evento dopo il timeout", function()
    rec:onSpellcast(7620)
    clock = 1010
    rec:onSpellcast(7620)
    rec:onLootReady(true, nil, { "Raw Brilliant Smallfish" })
    assert.equals(3, #timers) -- anche la cattura riprogramma il timer
    timers[1].fn() -- timer vecchi: non devono chiudere
    timers[2].fn()
    assert.equals(0, #events())
    clock = 1130
    assert.equals(120, timers[3].sec)
    timers[3].fn()
    assert.same({ "fishing" }, kinds())
    local e = events()[1]
    assert.equals(2, e.casts)
    assert.same({ "Raw Brilliant Smallfish" }, e.catches)
    assert.equals(1000, e.from)
    timers[3].fn() -- doppia chiamata: nessun secondo evento
    assert.equals(1, #events())
  end)

  it("pesca: catture senza lanci non spezzano lo span ogni 120 s", function()
    rec:onFishCatch("Fish A")
    assert.equals(1, #timers)
    clock = 1100
    rec:onFishCatch("Fish B")
    assert.equals(2, #timers)
    clock = 1120
    timers[1].fn() -- 120 s dalla prima cattura, ma solo 20 dall'ultima: resta aperto
    assert.equals(0, #events())
    clock = 1220
    timers[2].fn()
    assert.same({ "fishing" }, kinds())
    assert.same({ "Fish A", "Fish B" }, events()[1].catches)
    assert.equals(1000, events()[1].from)
    assert.equals(1100, events()[1].to)
  end)

  it("LOOT_CLOSED: la sorgente resta 2 s (autoloot veloce), poi non c'e' piu'", function()
    rec:onLootReady(false, "Murloc Forager")
    rec:onLootClosed()
    clock = 1001
    rec:onLootMsg("You receive loot: " .. RARE .. ".")
    assert.equals("Murloc Forager", events()[1].src)
  end)

  it("LOOT_CLOSED: 3 s dopo la sorgente e' scaduta", function()
    rec:onLootReady(false, "Murloc Forager")
    rec:onLootClosed()
    clock = 1003
    rec:onLootMsg("You receive loot: " .. RARE .. ".")
    assert.is_nil(events()[1].src)
  end)

  it("spell non di pesca ignorata", function()
    rec:onSpellcast(1)
    assert.equals(0, #timers)
  end)

  it("loot di pesca senza span aperto ne apre uno", function()
    rec:onLootReady(true, nil, { "Fish" })
    rec:flush()
    assert.equals("fishing", events()[1].k)
    assert.same({ "Fish" }, events()[1].catches)
  end)

  it("loot di pesca marca la sorgente come fishing", function()
    rec:onLootReady(true, nil, {})
    rec:onLootMsg("You receive loot: " .. RARE .. ".")
    assert.equals("fishing", events()[1].src)
  end)

  it("catches limitato a MAX_CATCHES", function()
    local names = {}
    for i = 1, 25 do names[i] = "F" .. i end
    rec:onLootReady(true, nil, names)
    rec:flush()
    assert.equals(20, #events()[1].catches)
  end)

  it("pesca seguita da combattimento: fishing prima di combat", function()
    rec:onSpellcast(7620)
    rec:onCombat(true)
    rec:onCombat(false)
    assert.same({ "fishing", "combat" }, kinds())
  end)

  it("combattimento 100 -> 12 -> 80", function()
    rec:onCombat(true)
    rec:onHealth(100, 100)
    rec:onHealth(12, 100)
    rec:onHealth(80, 100)
    clock = 1020
    rec:onCombat(false)
    local e = events()[1]
    assert.equals("combat", e.k)
    assert.equals(12, e.minHp)
    assert.equals(1000, e.from)
    assert.equals(1020, e.to)
  end)

  it("onHealth secret o max 0 ignorato", function()
    secret[5] = true
    rec:onCombat(true)
    rec:onHealth(5, 100)
    rec:onHealth(50, 0)
    rec:onCombat(false)
    assert.is_nil(events()[1].minHp)
  end)

  it("combattimento senza letture della vita: nessun campo minHp", function()
    rec:onCombat(true)
    rec:onCombat(false)
    assert.equals("combat", events()[1].k)
    assert.is_nil(events()[1].minHp)
  end)

  it("ingresso in combattimento al 40% senza danni: minHp 40", function()
    rec:onCombat(true)
    rec:onHealth(40, 100)
    rec:onCombat(false)
    assert.equals(40, events()[1].minHp)
  end)

  it("flush chiude combattimento aperto", function()
    rec:onCombat(true)
    rec:flush()
    assert.same({ "combat" }, kinds())
  end)

  it("level", function()
    rec:onLevel(12)
    assert.equals(12, events()[1].level)
  end)

  it("morte con respiro attivo", function()
    rec:onBreath(true)
    rec:onDead()
    assert.is_true(events()[1].breath)
    rec:onBreath(false)
    rec:onDead()
    assert.is_false(events()[2].breath)
  end)

  it("soldi: calo grande e piccolo", function()
    rec:onMoney(600000, "other")
    rec:onMoney(540000, "merchant")
    local e = events()[1]
    assert.equals("spend", e.k)
    assert.equals(60000, e.amount)
    assert.equals("merchant", e.where)
    rec:onMoney(539900, "other")
    assert.equals(1, #events())
    rec:onMoney(600000, "other") -- aumento
    assert.equals(1, #events())
  end)
end)
