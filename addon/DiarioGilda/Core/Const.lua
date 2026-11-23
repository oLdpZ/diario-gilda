-- Core/Const.lua: costanti dell'addon, nessuna dipendenza dal gioco.
--   DG.Const.VERSION, MAX_EVENTS (tetto eventi per sessione), KEEP_SECONDS (potatura sessioni vecchie),
--   FISHING_IDLE (secondi senza lanci per chiudere la pesca), GROUP_SETTLE (secondi di stabilità del gruppo),
--   SPEND_MIN (rame minimo per un evento spesa), MIN_QUALITY (qualità minima degli oggetti registrati),
--   GREEN_MAX_LEVEL (sotto questo livello si registrano anche i verdi),
--   MAX_DESC / MAX_OBJ / MAX_FINE (caratteri massimi dei testi di una missione consegnata),
--   MAX_CATCHES / MAX_ERRORS (tetti degli array), RELOAD_GAP (secondi tra due login per restare nella stessa sessione),
--   START_RETRY / START_TRIES (attesa e tentativi se al login nome o reame non sono ancora leggibili),
--   FISHING_SPELLS (set di spell id della pesca, da verificare in gioco con /diario debug).
local _, DG = ...

DG.Const = {
  VERSION = "0.2.5", -- uguale a "## Version:" del TOC (controllato dai test e dalla release)
  MAX_EVENTS = 5000,
  KEEP_SECONDS = 1209600,
  FISHING_IDLE = 120,
  GROUP_SETTLE = 5,
  SPEND_MIN = 50000,
  MIN_QUALITY = 3,
  GREEN_MAX_LEVEL = 20,
  MAX_CATCHES = 20,
  MAX_ERRORS = 20,
  RELOAD_GAP = 600,
  START_RETRY = 2, -- secondi tra due letture di nome e reame al login
  START_TRIES = 30, -- letture in piu' dopo la prima (circa un minuto)
  MAX_DESC = 600,
  MAX_OBJ = 300,
  MAX_FINE = 400,
  FISHING_SPELLS = { [7620] = true, [7731] = true, [7732] = true, [18248] = true, [131474] = true, [131490] = true },
}
