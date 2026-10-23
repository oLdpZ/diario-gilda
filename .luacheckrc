-- Configurazione luacheck. Lua 5.1 (WoW).
-- I read_globals si aggiungono task per task, uno per ogni API di Blizzard usata (ogni aggiunta è un diff da rivedere).
std = "lua51"
max_line_length = 120
codes = true

globals = { "DiarioGildaDB", "SLASH_DIARIO1", "SlashCmdList" }
read_globals = {}
