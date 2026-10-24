-- Core/Const.lua: costanti dell'addon, nessuna dipendenza dal gioco.
--   DG.Const.VERSION, MAX_EVENTS (tetto eventi per sessione), KEEP_SECONDS (potatura sessioni vecchie),
--   FISHING_IDLE (secondi senza lanci per chiudere la pesca), GROUP_SETTLE (secondi di stabilità del gruppo),
--   SPEND_MIN (rame minimo per un evento spesa), MIN_QUALITY (qualità minima degli oggetti registrati),
--   MAX_CATCHES / MAX_ERRORS (tetti degli array), RELOAD_GAP (secondi tra due login per restare nella stessa sessione),
--   FISHING_SPELLS (set di spell id della pesca, da verificare in gioco con /diario debug).
local _, DG = ...

DG.Const = {
  VERSION = "0.1.0-test", -- uguale a "## Version:" del TOC (controllato dai test e dalla release)
  MAX_EVENTS = 5000,
  KEEP_SECONDS = 1209600,
  FISHING_IDLE = 120,
  GROUP_SETTLE = 5,
  SPEND_MIN = 50000,
  MIN_QUALITY = 3,
  MAX_CATCHES = 20,
  MAX_ERRORS = 20,
  RELOAD_GAP = 600,
  FISHING_SPELLS = { [7620] = true, [7731] = true, [7732] = true, [18248] = true, [131474] = true, [131490] = true },
}
