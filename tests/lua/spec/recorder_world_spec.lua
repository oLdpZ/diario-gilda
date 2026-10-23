local H = require("helpers")

local GLOBALS = {
  DUEL_WINNER_KNOCKOUT = "%1$s has defeated %2$s in a duel",
  DUEL_WINNER_RETREAT = "%2$s has fled from %1$s in a duel",
}

describe("Recorder (boss, gruppo, missioni, duelli, zone)", function()
  local DG, J, db, rec, clock, timers, secret

  local function events() return J.current().events end
  local function fireLast() timers[#timers].fn() end

  before_each(function()
    clock = 1000
    timers = {}
    secret = {}
    DG = H.load({ "Core/Const.lua", "Core/Safe.lua", "Core/Journal.lua", "Core/Parse.lua", "Core/Recorder.lua" })
    DG.Safe.init(function(v) return secret[v] == true end)
    DG.Parse.init(GLOBALS)
    J = DG.Journal
    db = J.load(nil, { now = function() return clock end, where = function() return { z = "Westfall" } end })
    J.begin({ char = "Gino", realm = "R", class = "WARRIOR", race = "Human", level = 1, build = "1" }, false)
    rec = DG.Recorder.new(J, {
      now = function() return clock end,
      itemInfo = function() end,
      after = function(sec, fn) timers[#timers + 1] = { sec = sec, fn = fn } end,
    })
  end)

  it("boss vinto: from = inizio, ok true", function()
    rec:onEncounterStart(1234, "Edwin", 1, 5)
    clock = 1100
    rec:onEncounterEnd(1234, "Edwin", 1, 5, 1)
    local e = events()[1]
    assert.equals("encounter", e.k)
    assert.equals(1000, e.from)
    assert.equals(1100, e.to)
    assert.is_true(e.ok)
    assert.equals("Edwin", e.name)
    assert.equals(1, e.diff)
    assert.equals(5, e.size)
  end)

  it("fine senza inizio: from == t; wipe: ok false", function()
    rec:onEncounterEnd(9, "X", 1, 5, 0)
    local e = events()[1]
    assert.equals(e.t, e.from)
    assert.is_false(e.ok)
  end)

  it("nome secret: evento senza name", function()
    secret["Hidden"] = true
    rec:onEncounterEnd(9, "Hidden", 1, 5, 1)
    assert.equals(1, #events())
    assert.is_nil(events()[1].name)
    assert.equals(0, #db.errors)
  end)

  it("roster che cambia 3 volte: un solo group", function()
    rec:onRoster({ { name = "A" } })
    rec:onRoster({ { name = "A" }, { name = "B", guild = true } })
    rec:onRoster({ { name = "A" }, { name = "B", guild = true }, { name = "C" } })
    timers[1].fn()
    timers[2].fn()
    assert.equals(0, #events())
    assert.equals(5, timers[3].sec)
    fireLast()
    assert.equals(1, #events())
    assert.same({ "A", "B", "C" }, events()[1].members)
    assert.same({ "B" }, events()[1].guild)
  end)

  it("stesso roster (altro ordine): nessun nuovo evento; sciolto: members vuoto", function()
    rec:onRoster({ { name = "A" }, { name = "B" } })
    fireLast()
    rec:onRoster({ { name = "B" }, { name = "A" } })
    fireLast()
    assert.equals(1, #events())
    rec:onRoster({})
    fireLast()
    assert.equals(2, #events())
    assert.same({}, events()[2].members)
  end)

  it("nessun gruppo al login: nessun evento", function()
    rec:onRoster({})
    fireLast()
    assert.equals(0, #events())
  end)

  it("missioni", function()
    rec:onQuestDone(77, "Il Raccolto", 250, 120)
    rec:onQuestAbandon(78, "Altro")
    local a, b = events()[1], events()[2]
    assert.equals("quest", a.k)
    assert.equals("done", a.action)
    assert.equals(250, a.xp)
    assert.equals(120, a.money)
    assert.equals("abandon", b.action)
    assert.equals(78, b.id)
  end)

  it("duello", function()
    rec:onSystemMsg("Gino has defeated Bob in a duel")
    local e = events()[1]
    assert.equals("duel", e.k)
    assert.equals("Gino", e.winner)
    assert.equals("Bob", e.loser)
    assert.equals("knockout", e.how)
    rec:onSystemMsg("hello")
    assert.equals(1, #events())
  end)

  it("messaggio di sistema secret: nessun evento e nessun errore", function()
    local msg = "Gino has defeated Bob in a duel"
    secret[msg] = true
    rec:onSystemMsg(msg)
    assert.equals(0, #events())
    assert.equals(0, #db.errors)
  end)

  it("zona: first poi non first; nil o secret nessun evento", function()
    rec:onZone("Westfall")
    rec:onZone("Westfall")
    assert.is_true(events()[1].first)
    assert.is_false(events()[2].first)
    secret["Hid"] = true
    rec:onZone(nil)
    rec:onZone("Hid")
    assert.equals(2, #events())
    assert.equals(0, #db.errors)
  end)
end)
