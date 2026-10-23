local H = require("helpers")

local GLOBALS = {
  LOOT_ITEM_SELF = "You receive loot: %s.",
  LOOT_ITEM_SELF_MULTIPLE = "You receive loot: %sx%d.",
  LOOT_ITEM_PUSHED_SELF = "You receive item: %s.",
  LOOT_ITEM_PUSHED_SELF_MULTIPLE = "You receive item: %sx%d.",
  LOOT_ITEM_CREATED_SELF = "You create: %s.",
  LOOT_ITEM_CREATED_SELF_MULTIPLE = "You create: %sx%d.",
  DUEL_WINNER_KNOCKOUT = "%1$s has defeated %2$s in a duel",
  DUEL_WINNER_RETREAT = "%2$s has fled from %1$s in a duel",
}

local LINK = "|cff0070dd|Hitem:2042::::::::|h[Staff of Westfall]|h|r"

describe("Parse", function()
  local Parse
  before_each(function()
    Parse = H.load({ "Core/Const.lua", "Core/Safe.lua", "Core/Parse.lua" }).Parse
    Parse.init(GLOBALS)
  end)

  describe("pattern", function()
    it("ancora ed esegue l'escape dei caratteri magici", function()
      local p = Parse.pattern("a (b)-c %s.")
      assert.equals("x", ("a (b)-c x."):match(p))
      assert.is_nil(("zz a (b)-c x."):match(p))
    end)
  end)

  describe("loot", function()
    it("bottino singolo", function()
      local r = Parse.loot("You receive loot: " .. LINK .. ".")
      assert.same({ kind = "loot", link = LINK, n = 1 }, r)
    end)
    it("bottino multiplo", function()
      local r = Parse.loot("You receive loot: " .. LINK .. "x3.")
      assert.same({ kind = "loot", link = LINK, n = 3 }, r)
    end)
    it("oggetto ricevuto in altro modo", function()
      assert.equals("loot", Parse.loot("You receive item: " .. LINK .. "."). kind)
      assert.equals(2, Parse.loot("You receive item: " .. LINK .. "x2.").n)
    end)
    it("creazione", function()
      local r = Parse.loot("You create: " .. LINK .. ".")
      assert.equals("craft", r.kind)
      assert.equals(LINK, r.link)
      assert.equals(4, Parse.loot("You create: " .. LINK .. "x4.").n)
    end)
    it("scarta il bottino degli altri", function()
      assert.is_nil(Parse.loot("Bob receives loot: " .. LINK .. "."))
      assert.is_nil(Parse.loot("Bob creates: " .. LINK .. "."))
    end)
    it("nomi con parentesi e trattini", function()
      local link = "|cffffffff|Hitem:1::::::::|h[Sword (Old) of-Doom]|h|r"
      assert.equals(link, Parse.loot("You receive loot: " .. link .. ".").link)
    end)
    it("non solleva mai errori", function()
      assert.is_nil(Parse.loot(nil))
      assert.is_nil(Parse.loot(42))
      assert.is_nil(Parse.loot({}))
      assert.is_nil(Parse.loot(""))
    end)
  end)

  describe("duel", function()
    it("knockout", function()
      assert.same({ winner = "Gino", loser = "Bob", how = "knockout" },
        Parse.duel("Gino has defeated Bob in a duel"))
    end)
    it("ritirata con argomenti invertiti", function()
      assert.same({ winner = "Gino", loser = "Bob", how = "retreat" },
        Parse.duel("Bob has fled from Gino in a duel"))
    end)
    it("altri messaggi e input non validi", function()
      assert.is_nil(Parse.duel("Gino says hi"))
      assert.is_nil(Parse.duel(nil))
      assert.is_nil(Parse.duel(5))
    end)
    it("formati non posizionali: ordine di apparizione", function()
      Parse.init({ DUEL_WINNER_KNOCKOUT = "%s has defeated %s in a duel" })
      assert.same({ winner = "A", loser = "B", how = "knockout" }, Parse.duel("A has defeated B in a duel"))
      assert.is_nil(Parse.duel("Bob has fled from Gino in a duel"))
    end)
  end)

  it("ignora le GlobalStrings mancanti", function()
    Parse.init({})
    assert.is_nil(Parse.loot("You receive loot: " .. LINK .. "."))
    assert.is_nil(Parse.duel("Gino has defeated Bob in a duel"))
  end)
end)
