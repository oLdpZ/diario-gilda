-- Configurazione luacheck. Lua 5.1 (WoW).
-- I read_globals si aggiungono task per task, uno per ogni API di Blizzard usata (ogni aggiunta è un diff da rivedere).
std = "lua51"
max_line_length = 120
codes = true

globals = { "DiarioGildaDB", "SLASH_DIARIO1", "SlashCmdList" }
read_globals = {
  "AbandonQuest",
  "C_Item",
  "C_Map",
  "C_QuestLog",
  "C_Spell",
  "C_Timer",
  "CreateFrame",
  "GetBuildInfo",
  "GetGuildInfo",
  "GetInstanceInfo",
  "GetLootSlotInfo",
  "GetMoney",
  "GetNumLootItems",
  "GetRealmName",
  "GetSubZoneText",
  "GetZoneText",
  "IsFishingLoot",
  "IsInInstance",
  "IsInRaid",
  "LoggingCombat",
  "UnitClass",
  "UnitExists",
  "UnitHealth",
  "UnitHealthMax",
  "UnitIsDead",
  "UnitIsInMyGuild",
  "UnitIsUnit",
  "UnitLevel",
  "UnitName",
  "UnitRace",
  "hooksecurefunc",
  "issecretvalue",
  "time",
}
