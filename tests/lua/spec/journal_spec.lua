local H = require("helpers")

describe("Journal", function()
  local DG, J, db, clock, where
  local gino = {
    char = "Gino", realm = "R", guild = "Gilda", class = "WARRIOR", race = "Human", level = 1, build = "1.60.1",
  }

  local function fresh(saved)
    DG = H.load({ "Core/Const.lua", "Core/Safe.lua", "Core/Journal.lua" })
    J = DG.Journal
    db = J.load(saved, { now = function() return clock end, where = function() return where end })
  end

  before_each(function()
    clock = 1000
    where = { z = "Westfall", s = "Longshore", m = 1436 }
    fresh(nil)
  end)

  it("begin crea la sessione con id realm-char-start", function()
    local s = J.begin(gino, false)
    assert.equals("R-Gino-1000", s.id)
    assert.equals(1000, s.start)
    assert.is_nil(s.stop)
    assert.equals(0, s.dropped)
    assert.equals(1, db.v)
    assert.equals(s, db.sessions[1])
    assert.equals(s, J.current())
  end)

  it("add aggiunge t, k e i campi di where senza sovrascrivere", function()
    J.begin(gino, false)
    clock = 1010
    local ev = J.add("level", { level = 2, z = "Altrove" })
    assert.equals(1010, ev.t)
    assert.equals("level", ev.k)
    assert.equals(2, ev.level)
    assert.equals("Altrove", ev.z)
    assert.equals("Longshore", ev.s)
    assert.equals(1436, ev.m)
    assert.is_nil(ev.x)
    assert.equals(1, #J.current().events)
  end)

  it("add senza sessione restituisce nil", function()
    assert.is_nil(J.add("level", {}))
  end)

  it("/reload entro RELOAD_GAP dopo finish riapre la stessa sessione", function()
    local s = J.begin(gino, false)
    J.add("zone", {})
    clock = 1100
    J.finish()
    assert.equals(1100, s.stop)
    clock = 1500
    local again = J.begin(gino, true)
    assert.equals(s, again)
    assert.equals("R-Gino-1000", again.id)
    assert.is_nil(again.stop)
    assert.equals(1, #db.sessions)
  end)

  it("/reload oltre RELOAD_GAP apre una nuova sessione", function()
    J.begin(gino, false)
    clock = 1100
    J.finish()
    clock = 1100 + 601
    local s = J.begin(gino, true)
    assert.equals(2, #db.sessions)
    assert.equals("R-Gino-1701", s.id)
  end)

  it("/reload dopo un crash (stop nil) continua la stessa sessione", function()
    local s = J.begin(gino, false)
    clock = 5000
    assert.equals(s, J.begin(gino, true))
    assert.equals(1, #db.sessions)
  end)

  it("login normale dopo crash chiude la vecchia con stop = ultimo evento", function()
    local old = J.begin(gino, false)
    clock = 1500
    J.add("zone", {})
    clock = 9000
    local s = J.begin(gino, false)
    assert.equals(1500, old.stop)
    assert.is_nil(s.stop)
    assert.equals("R-Gino-9000", s.id)
    assert.equals(2, #db.sessions)
  end)

  it("crash senza eventi: stop = start", function()
    local old = J.begin(gino, false)
    clock = 9000
    J.begin(gino, false)
    assert.equals(1000, old.stop)
  end)

  it("il 5001esimo evento e' nil, dropped sale e finish funziona", function()
    local s = J.begin(gino, false)
    for _ = 1, 5000 do
      assert.is_truthy(J.add("zone", {}))
    end
    assert.is_nil(J.add("zone", {}))
    assert.equals(1, s.dropped)
    assert.equals(5000, #s.events)
    clock = 2000
    J.finish()
    assert.equals(2000, s.stop)
  end)

  it("pota le sessioni piu' vecchie di 14 giorni al begin", function()
    clock = 1000000
    J.begin(gino, false)
    J.finish()
    clock = 1000000 + 15 * 86400
    J.begin(gino, false)
    assert.equals(1, #db.sessions)
    assert.equals(clock, db.sessions[1].start)
  end)

  it("firstVisit e' vero solo la prima volta e lo segna in seen", function()
    J.begin(gino, false)
    assert.is_true(J.firstVisit("Westfall"))
    assert.is_false(J.firstVisit("Westfall"))
    assert.is_true(db.seen["R-Gino"]["Westfall"])
  end)

  it("un altro personaggio ha sessioni e seen separati", function()
    local a = J.begin(gino, false)
    J.firstVisit("Westfall")
    clock = 1100
    local b = J.begin({ char = "Pino", realm = "R" }, false)
    assert.is_nil(a.stop)
    assert.is_true(J.firstVisit("Westfall"))
    assert.equals(2, #db.sessions)
    assert.not_equals(a.id, b.id)
    -- il /reload di Gino riapre la sua sessione, non quella di Pino
    assert.equals(a, J.begin(gino, true))
  end)

  it("load completa una tabella salvata incompleta e la riusa", function()
    local saved = { sessions = {} }
    fresh(saved)
    assert.equals(saved, db)
    assert.same({}, db.seen)
    assert.same({}, db.errors)
  end)

  it("logError tiene gli ultimi MAX_ERRORS", function()
    for i = 1, 25 do J.logError("e" .. i) end
    assert.equals(20, #db.errors)
    assert.equals("e6", db.errors[1])
    assert.equals("e25", db.errors[20])
  end)

  it("un where che va in errore non perde l'evento", function()
    J.begin(gino, false)
    where = nil
    local ev = J.add("note", { text = "x" })
    assert.equals("x", ev.text)
  end)
end)
