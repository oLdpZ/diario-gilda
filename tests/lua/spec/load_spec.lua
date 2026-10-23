local H = require("helpers")

describe("struttura dell'addon", function()
  it("carica Const", function()
    assert.equals("0.1.0", H.load({ "Core/Const.lua" }).Const.VERSION)
  end)

  it("il TOC dichiara interfaccia e SavedVariables ed elenca file esistenti", function()
    local toc = H.readFile(H.ADDON_DIR .. "/DiarioGilda.toc")
    assert.truthy(toc:find("## Interface: 16001", 1, true))
    assert.truthy(toc:find("## SavedVariables: DiarioGildaDB", 1, true))
    local count = 0
    for line in toc:gmatch("[^\r\n]+") do
      if not line:find("^#") and line:find("%S") then
        count = count + 1
        local path = line:gsub("\\", "/"):gsub("%s+$", "")
        local f = io.open(H.ADDON_DIR .. "/" .. path, "rb")
        assert.truthy(f, "file nel TOC mancante: " .. path)
        f:close()
      end
    end
    assert.truthy(count >= 1)
  end)
end)
