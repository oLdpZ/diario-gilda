local H = require("helpers")

describe("struttura dell'addon", function()
  it("carica Const con la stessa versione del TOC", function()
    -- Una sola versione: la release controlla il TOC, l'addon scrive Const.VERSION in ogni sessione.
    local toc = H.readFile(H.ADDON_DIR .. "/DiarioGilda.toc")
    local version
    for line in toc:gmatch("[^\r\n]+") do
      version = version or line:match("^## Version:%s*(%S+)")
    end
    assert.truthy(version and version ~= "", "## Version mancante nel TOC")
    assert.equals(version, H.load({ "Core/Const.lua" }).Const.VERSION)
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
