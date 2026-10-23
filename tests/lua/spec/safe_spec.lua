local H = require("helpers")

describe("Safe", function()
  local Safe
  before_each(function()
    Safe = H.load({ "Core/Const.lua", "Core/Safe.lua" }).Safe
  end)

  describe("con init", function()
    before_each(function()
      Safe.init(function(v) return v == "SEGRETO" end)
    end)

    it("get scarta i secret e lascia passare il resto", function()
      assert.is_nil(Safe.get("SEGRETO"))
      assert.equals("Westfall", Safe.get("Westfall"))
      assert.equals(0, Safe.get(0))
    end)

    it("str accetta solo stringhe non vuote e non secret", function()
      assert.equals("Westfall", Safe.str("Westfall"))
      assert.is_nil(Safe.str(""))
      assert.is_nil(Safe.str(12))
      assert.is_nil(Safe.str(nil))
      assert.is_nil(Safe.str("SEGRETO"))
    end)

    it("num accetta solo numeri non secret", function()
      assert.is_nil(Safe.num("3"))
      assert.equals(3, Safe.num(3))
      assert.is_nil(Safe.num(nil))
    end)
  end)

  it("senza init nulla è secret", function()
    Safe.init(nil)
    assert.equals("SEGRETO", Safe.get("SEGRETO"))
  end)

  it("call restituisce i valori, o nil se la funzione va in errore", function()
    assert.is_nil(Safe.call(function() error("x") end))
    local a, b = Safe.call(function(x, y) return x + y, "ok" end, 1, 2)
    assert.equals(3, a)
    assert.equals("ok", b)
  end)

  it("call conserva i ritorni dopo un nil (es. GetInstanceInfo)", function()
    local a, b, c = Safe.call(function() return 1, nil, 3 end)
    assert.equals(1, a)
    assert.is_nil(b)
    assert.equals(3, c)
    assert.equals(3, select("#", Safe.call(function() return 1, nil, 3 end)))
    -- con un nil in coda la lunghezza della tabella dei ritorni non e' affidabile: vanno contati con select
    local _, _, c2 = Safe.call(function() return 1, nil, 3, nil end)
    assert.equals(3, c2)
    assert.equals(4, select("#", Safe.call(function() return 1, nil, 3, nil end)))
  end)
end)
