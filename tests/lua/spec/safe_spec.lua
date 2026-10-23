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
end)
