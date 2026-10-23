# Diario di Gilda — Piano 1: Addon `DiarioGilda` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un addon per il client WoW Forever che registra i fatti di gioco di ogni sessione in SavedVariables, nel formato che il programmino (Piano 2) legge.

**Architecture:** Logica pura in `Core/` (nessun accesso a frame o API di gioco: tutto arriva da dipendenze iniettate), testata con `busted`; `Main.lua` è l'unico file che tocca il gioco: crea un frame, registra gli eventi, legge le API e chiama `Core/Recorder`. Stessa impostazione del progetto di riferimento `C:\Users\old_p\Documents\wow forever traduzione` (vedi `addon/OldpZ_ForeverITA/Core/Collector.lua` e `tests/lua/spec/helpers.lua` come modello di stile e di harness).

**Tech Stack:** Lua 5.1 (WoW), busted + luacheck in WSL (`archlinux`, come `scripts/wsl-setup.sh` del progetto di riferimento).

**Spec:** `docs/superpowers/specs/2026-10-23-diario-di-gilda-design.md`

## Global Constraints

- Client unico: WoW Forever, `## Interface: 16001`. Nessun supporto ad altre versioni.
- Nome addon e cartella: `DiarioGilda`; SavedVariables account-wide: `DiarioGildaDB`. Slash: `/diario`.
- Nessuna lettura della chat dei giocatori (CHAT_MSG_SAY/PARTY/GUILD/WHISPER/…). Solo `CHAT_MSG_LOOT` e `CHAT_MSG_SYSTEM`.
- Ogni valore per cui `issecretvalue(v)` è vero viene scartato senza leggerlo, confrontarlo o concatenarlo (vedi `docs/architecture/client-limits.md` del progetto di riferimento).
- Mai toccare frame sicuri, mai `hooksecurefunc` su funzioni protette diverse da `AbandonQuest`; nessun errore Lua può uscire dagli handler (ogni handler è avvolto in `pcall`, l'errore finisce in `DiarioGildaDB.errors`, max 20).
- Tetti: `MAX_EVENTS = 5000` per sessione (oltre: si conta in `dropped`), sessioni più vecchie di `KEEP_SECONDS = 14 * 86400` vengono potate al login.
- Orari: `time()` del PC (epoch, secondi), lo stesso orologio del `WoWCombatLog.txt`.
- Commenti e messaggi al giocatore in italiano; identificatori in inglese.
- Test: `bash scripts/test-lua.sh` (esegue busted e luacheck in WSL; il percorso del progetto contiene spazi, quindi niente `make`).

## Contratto dati (`DiarioGildaDB`, versione 1)

Questo è il contratto con il Piano 2. Task 1 lo copia in `docs/formato-dati.md`; ogni cambiamento successivo aggiorna quel file.

```lua
DiarioGildaDB = {
  v = 1,
  sessions = {            -- array, in ordine di inizio
    { id = "Realm-Char-1793000000", char = "Gino", realm = "Realm", guild = "Nome gilda" --[[o nil]],
      class = "WARRIOR", race = "Human", level = 12, build = "1.60.1.70338", addon = "0.1.0",
      start = 1793000000, stop = 1793007200 --[[nil se in corso o crash]], dropped = 0,
      events = { { t = 1793000123, k = "zone", z = "Westfall", s = "Longshore", m = 1436, x = 0.512, y = 0.301, ... } },
    },
  },
  seen = { ["Realm-Char"] = { ["Westfall"] = true } },   -- zone già visitate (per `first`)
  errors = { "…" },                                        -- ultimi 20 errori dell'addon
}
```

Campi comuni a ogni evento: `t` (epoch), `k` (tipo), `z` zona, `s` sottozona, `m` uiMapID, `x`/`y` posizione 0–1 a 3 decimali (assenti in istanza), `inst` nome istanza se in istanza.

| `k` | campi propri | quando |
|---|---|---|
| `zone` | `first` (bool) | cambio zona (`ZONE_CHANGED_NEW_AREA`) e al login |
| `loot` | `item`, `id`, `q`, `n`, `src` (`"fishing"`, nome del mob, o assente) | oggetto ricevuto di qualità `q >= 3` |
| `craft` | `item`, `id`, `q`, `n` | oggetto creato di qualità `q >= 3` |
| `fishing` | `from`, `to`, `casts`, `catches` (array di nomi, max 20) | chiuso dopo 120 s senza lanci, al logout o all'inizio di un combattimento |
| `level` | `level` | level up |
| `death` | `breath` (bool: timer del respiro attivo) | morte del giocatore |
| `combat` | `from`, `to`, `minHp` (0–100) | fine combattimento del giocatore |
| `encounter` | `id`, `name`, `diff`, `size`, `ok` (bool), `from`, `to` | fine incontro con un boss |
| `group` | `members` (nomi), `guild` (nomi in gilda) | composizione del gruppo cambiata e stabile da 5 s; gruppo vuoto = `members = {}` |
| `quest` | `id`, `title`, `action` (`"done"`/`"abandon"`), `xp`, `money` | missione consegnata o abbandonata |
| `duel` | `winner`, `loser`, `how` (`"knockout"`/`"retreat"`) | messaggio di sistema di fine duello |
| `spend` | `amount` (rame), `where` (`"merchant"`/`"auction"`/`"trainer"`/`"other"`) | calo dei soldi `>= 50000` rame (5 oro) in un colpo |
| `note` | `text` | solo messaggi dell'addon stesso (es. registro combattimenti spento) |

## Review Focus

- `/reload` a metà serata: deve continuare la stessa sessione, non aprirne una nuova (test in Task 3).
- Valori secret in dungeon (nomi in chat di sistema, nomi del gruppo, nome del boss): l'evento si registra senza quel campo, non salta né va in errore (test in Task 2, 4 e 6).
- Messaggi di bottino di altri giocatori del gruppo ("Bob receives loot…") non devono finire come bottino del giocatore (test in Task 4).
- Una serata lunghissima (tetto di 5000 eventi) non deve crescere senza limiti né perdere `stop` (test in Task 3).
- Il gioco va in crash: la sessione resta senza `stop` e al login successivo deve essere chiusa con `stop = ultimo evento`, non lasciata aperta per sempre (test in Task 3).

---

### Task 1: Struttura, harness dei test, contratto dati

**Files:**
- Create: `addon/DiarioGilda/DiarioGilda.toc`, `addon/DiarioGilda/Core/Const.lua`
- Create: `.busted`, `.luacheckrc`, `scripts/test-lua.sh`, `tests/lua/spec/helpers.lua`, `tests/lua/spec/load_spec.lua`
- Create: `docs/formato-dati.md`, `.gitignore`

**Interfaces:**
- Produces: namespace `DG` (secondo valore di `...` in ogni file); `DG.Const = { VERSION = "0.1.0", MAX_EVENTS = 5000, KEEP_SECONDS = 1209600, FISHING_IDLE = 120, GROUP_SETTLE = 5, SPEND_MIN = 50000, MIN_QUALITY = 3, MAX_CATCHES = 20, MAX_ERRORS = 20, RELOAD_GAP = 600, FISHING_SPELLS = {[7620]=true,[7731]=true,[7732]=true,[18248]=true,[131474]=true,[131490]=true} }` (gli id della pesca sono da verificare in gioco con `/diario debug`, Task 7).
- Produces: `helpers.load(files) -> DG` carica i file elencati (percorsi relativi a `addon/DiarioGilda/`) con `loadfile(path)("DiarioGilda", ns)`, in un ambiente dove i globali di gioco non esistono.

- [ ] **Step 1:** Scrivi `tests/lua/spec/load_spec.lua`: `it("carica Const", …)` con `assert.equals("0.1.0", H.load({"Core/Const.lua"}).Const.VERSION)` e un test che legge il TOC e verifica `## Interface: 16001`, `## SavedVariables: DiarioGildaDB` e che ogni file elencato esista.
- [ ] **Step 2:** `bash scripts/test-lua.sh` → FAIL (file mancanti). Lo script: `wsl -d archlinux -- bash -lc 'cd "$(wslpath "<radice>")" && eval "$(luarocks --lua-version=5.1 --tree=$HOME/.luarocks path)" && busted && luacheck addon tests'`, con la radice ricavata da `dirname "$0"/..`. Se busted/luacheck mancano in WSL, installali come fa `scripts/wsl-setup.sh` del progetto di riferimento (`luarocks --lua-version=5.1 install busted luacheck`).
- [ ] **Step 3:** Crea TOC (`## Title: Diario di Gilda`, `## Notes: Registra le imprese per il diario della gilda`, `## Author: oLd_pZ`, `## Version: 0.1.0`, `## Interface: 16001`, `## SavedVariables: DiarioGildaDB`, poi `Core\Const.lua`), `Const.lua`, `.busted` (ROOT `tests/lua/spec`, pattern `_spec`), `.luacheckrc` (`std = "lua51"`, `max_line_length = 120`, `globals = {"DiarioGildaDB", "SLASH_DIARIO1", "SlashCmdList"}`; i `read_globals` si aggiungono task per task, uno per API usata), `helpers.lua`, `docs/formato-dati.md` (copia del Contratto dati qui sopra), `.gitignore` (`*.out`, `dist/`).
- [ ] **Step 4:** `bash scripts/test-lua.sh` → PASS, luacheck 0 warnings.
- [ ] **Step 5:** `git add -A && git commit -m "feat(addon): struttura, test e contratto dati"`

### Task 2: `Core/Safe.lua` — scarto dei valori secret

**Files:** Create `addon/DiarioGilda/Core/Safe.lua`, `tests/lua/spec/safe_spec.lua`; Modify TOC.

**Interfaces:**
- Produces: `DG.Safe.init(isSecret)` (funzione o nil: senza funzione nulla è secret); `DG.Safe.get(v) -> v | nil` (nil se secret); `DG.Safe.str(v) -> string | nil` (nil se secret, non stringa o vuota); `DG.Safe.num(v) -> number | nil`; `DG.Safe.call(fn, ...) -> ...` (pcall: in errore restituisce nil).

- [ ] **Step 1:** Test: con `isSecret = function(v) return v == "SEGRETO" end`: `get("SEGRETO") == nil`, `str("Westfall") == "Westfall"`, `str("") == nil`, `str(12) == nil`, `num("3") == nil`, `num(3) == 3`; senza init `get("SEGRETO") == "SEGRETO"`; `call(function() error("x") end) == nil`.
- [ ] **Step 2:** Esegui → FAIL. **Step 3:** Implementa. **Step 4:** Esegui → PASS.
- [ ] **Step 5:** Commit `feat(addon): scarto dei valori secret`.

### Task 3: `Core/Journal.lua` — sessioni ed eventi

**Files:** Create `addon/DiarioGilda/Core/Journal.lua`, `tests/lua/spec/journal_spec.lua`; Modify TOC.

**Interfaces:**
- Consumes: `DG.Const`.
- Produces:
  - `DG.Journal.load(saved, deps) -> db`: `saved` è `DiarioGildaDB` (o nil), restituisce la tabella da rimettere in `DiarioGildaDB`. `deps = { now() -> epoch, where() -> {z,s,m,x,y,inst} }`.
  - `Journal.begin(info, isReload)`: `info = {char, realm, guild, class, race, level, build}`. Se `isReload` e l'ultima sessione dello stesso `realm-char` ha `stop == nil` o `now - stop <= RELOAD_GAP`, la riapre (`stop = nil`); altrimenti chiude ogni sessione aperta di quel personaggio con `stop = t dell'ultimo evento (o start)` e ne crea una nuova con `id = realm .. "-" .. char .. "-" .. start`. Pota le sessioni con `start < now - KEEP_SECONDS`.
  - `Journal.add(k, fields) -> event | nil`: copia `fields`, aggiunge `t`, `k` e i campi di `where()` non già presenti; oltre `MAX_EVENTS` incrementa `dropped` e restituisce nil. Senza sessione aperta restituisce nil.
  - `Journal.finish()`: `stop = now()`.
  - `Journal.firstVisit(zone) -> bool`: vero la prima volta per quel personaggio, e lo segna in `seen`.
  - `Journal.current() -> session | nil`; `Journal.logError(msg)` (tieni gli ultimi `MAX_ERRORS`).

- [ ] **Step 1:** Test (orologio finto): `begin` crea sessione con id `"R-Gino-1000"`; `add("level", {level=2})` ha `t`, `k`, `z` da `where`; `/reload` entro 600 s dopo `finish` riapre la stessa sessione (stesso id, `stop == nil`); login normale dopo un crash (sessione con `stop == nil`, ultimo evento a t=1500) chiude la vecchia con `stop == 1500` e ne apre una nuova; 5001° evento → nil e `dropped == 1`, `finish` funziona comunque; sessione di 15 giorni fa potata al `begin`; `firstVisit("Westfall")` true poi false; un personaggio diverso sullo stesso account ha sessioni separate.
- [ ] **Step 2:** Esegui → FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(addon): diario delle sessioni`.

### Task 4: `Core/Parse.lua` — messaggi di bottino, creazione e duello

**Files:** Create `addon/DiarioGilda/Core/Parse.lua`, `tests/lua/spec/parse_spec.lua`; Modify TOC.

**Interfaces:**
- Produces:
  - `DG.Parse.pattern(fmt) -> luaPattern`: trasforma una GlobalString (`%s`, `%d`, `%1$s`, `%2$s`) in un pattern ancorato con catture, facendo l'escape dei caratteri magici.
  - `DG.Parse.init(globals)`: `globals` è una tabella con le GlobalStrings `LOOT_ITEM_SELF`, `LOOT_ITEM_SELF_MULTIPLE`, `LOOT_ITEM_PUSHED_SELF`, `LOOT_ITEM_PUSHED_SELF_MULTIPLE`, `LOOT_ITEM_CREATED_SELF`, `LOOT_ITEM_CREATED_SELF_MULTIPLE`, `DUEL_WINNER_KNOCKOUT`, `DUEL_WINNER_RETREAT` (quelle mancanti vengono ignorate).
  - `DG.Parse.loot(msg) -> { kind = "loot"|"craft", link = string, n = number } | nil`
  - `DG.Parse.duel(msg) -> { winner, loser, how } | nil` (in `DUEL_WINNER_*` il primo `%s` è il vincitore).

- [ ] **Step 1:** Test con le stringhe inglesi del client: `LOOT_ITEM_SELF = "You receive loot: %s."`, `LOOT_ITEM_SELF_MULTIPLE = "You receive loot: %sx%d."`, `LOOT_ITEM_PUSHED_SELF = "You receive item: %s."`, `LOOT_ITEM_CREATED_SELF = "You create: %s."`, `DUEL_WINNER_KNOCKOUT = "%1$s has defeated %2$s in a duel"`, `DUEL_WINNER_RETREAT = "%2$s has fled from %1$s in a duel"`. Asserzioni: `"You receive loot: |cff0070dd|Hitem:2042::::::::|h[Staff of Westfall]|h|r."` → `kind="loot", n=1`, link intatto; `…x3.` → `n=3`; `"You create: |cff…|h[X]|h|r."` → `kind="craft"`; `"Bob receives loot: …"` → nil; testo con `(` `)` `-` nel nome dell'oggetto funziona; `"Gino has defeated Bob in a duel"` → winner Gino; la variante retreat con argomenti invertiti → winner = `%1$s`, `how="retreat"`.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(addon): lettura dei messaggi di bottino e duello`.

### Task 5: `Core/Recorder.lua` (parte 1) — bottino, pesca, livelli, morti, soldi, combattimento

**Files:** Create `addon/DiarioGilda/Core/Recorder.lua`, `tests/lua/spec/recorder_items_spec.lua`; Modify TOC.

**Interfaces:**
- Consumes: `DG.Journal`, `DG.Parse`, `DG.Safe`, `DG.Const`.
- Produces: `DG.Recorder.new(journal, deps) -> rec`. `deps = { now(), itemInfo(link) -> id, name, quality | nil, after(seconds, fn) }`. Metodi (tutti senza valore di ritorno, ognuno avvolto in pcall che passa l'errore a `journal.logError`):
  - `rec:onLootMsg(msg)`: `Parse.loot`; con `itemInfo` scarta `q < MIN_QUALITY`; `src` = sorgente dell'ultimo `onLootReady` se avvenuto entro 10 s.
  - `rec:onLootReady(isFishing, sourceName)`: memorizza la sorgente (`"fishing"` o il nome); se `isFishing`, aggiunge i nomi degli oggetti alla pesca in corso (`rec:onFishCatch(name)`).
  - `rec:onSpellcast(spellId)`: se l'id è in `FISHING_SPELLS` apre o estende lo span di pesca (`casts + 1`); lo chiude dopo `FISHING_IDLE` s senza lanci (`deps.after`), all'inizio di un combattimento o in `rec:flush()`.
  - `rec:onLevel(level)`, `rec:onDead()` (`breath` = stato del timer respiro), `rec:onBreath(active)`.
  - `rec:onMoney(money, where)`: confronta con il valore precedente; un calo `>= SPEND_MIN` → `spend`.
  - `rec:onCombat(inCombat)`, `rec:onHealth(cur, max)`: durante il combattimento tiene `minHp = floor(100*cur/max)`; alla fine scrive `combat {from,to,minHp}`. Valori secret o `max == 0` vengono ignorati.
  - `rec:flush()`: chiude gli span aperti (pesca, combattimento) — chiamato al logout.

- [ ] **Step 1:** Test (journal reale con orologio finto, `after` che salva le callback): loot raro con mob → evento `loot` con `src="Murloc Forager"`, `q=3`; loot verde → nessun evento; due lanci di pesca e una cattura → dopo la callback dei 120 s un solo evento `fishing` con `casts=2`, `catches={"Raw Brilliant Smallfish"}`; pesca seguita da `onCombat(true)` → `fishing` chiuso prima del combattimento (ordine degli eventi: fishing, combat); combattimento con vita 100→12→80 → `combat.minHp == 12`; `onHealth` con valore secret ignorato; morte con timer del respiro attivo → `death.breath == true`; soldi 600000→540000 (60000 rame) in `merchant` → `spend` di 60000; calo di 100 rame → nulla; `itemInfo` che va in errore → nessun evento, errore in `errors`.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(addon): bottino, pesca, morti, soldi e combattimenti`.

### Task 6: `Core/Recorder.lua` (parte 2) — boss, gruppo, missioni, duelli, zone

**Files:** Modify `addon/DiarioGilda/Core/Recorder.lua`; Create `tests/lua/spec/recorder_world_spec.lua`.

**Interfaces:**
- Produces (metodi di `rec`):
  - `rec:onEncounterStart(id, name, diff, size)`, `rec:onEncounterEnd(id, name, diff, size, success)` → `encounter` con `from` dell'inizio corrispondente (o `t` se manca), `ok = success == 1`. Nome secret → campo `name` assente.
  - `rec:onRoster(members)`: `members = { {name=, guild=bool}, … }` (nomi secret già scartati da Main); dopo `GROUP_SETTLE` s senza altri cambi, se l'insieme dei nomi è diverso dall'ultimo registrato scrive `group`.
  - `rec:onQuestDone(id, title, xp, money)`, `rec:onQuestAbandon(id, title)`.
  - `rec:onSystemMsg(msg)`: `Parse.duel` → `duel`.
  - `rec:onZone(zone)`: `zone` con `first = journal.firstVisit(zone)`; zona nil o secret → nessun evento.

- [ ] **Step 1:** Test: start+end con success 1 → `encounter.ok == true`, `from` = t dell'inizio; end senza start → `from == t`; nome secret → nessun campo `name`, evento presente; roster che cambia 3 volte in 2 s → un solo `group` dopo 5 s; lo stesso roster ripetuto → nessun nuovo evento; gruppo sciolto → `members = {}`; duello → `duel` con vincitore; prima visita a Westfall → `first=true`, seconda → `first=false`.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(addon): boss, gruppi, missioni, duelli e zone`.

### Task 7: `Main.lua` — collegamento al gioco, `/diario`, registro combattimenti

**Files:** Create `addon/DiarioGilda/Main.lua`, `tests/lua/spec/main_spec.lua`; Modify TOC (`Main.lua` per ultimo), `.luacheckrc` (`read_globals` delle API usate qui sotto).

**Interfaces:**
- Consumes: tutto il Core.
- Produces: comportamento in gioco. Mappa eventi → metodi:
  - `ADDON_LOADED` (il proprio) → `Safe.init(issecretvalue)`, `Parse.init(_G)`, `DiarioGildaDB = Journal.load(DiarioGildaDB, deps)`.
  - `PLAYER_ENTERING_WORLD(isLogin, isReload)` se `isLogin or isReload` → `Journal.begin(info, isReload)`; info da `UnitName("player")`, `GetRealmName()`, `GetGuildInfo("player")`, `UnitClass` (secondo valore), `UnitRace` (secondo valore), `UnitLevel`, `GetBuildInfo()`; poi `rec:onZone(GetZoneText())` e attivazione registro combattimenti: `if not LoggingCombat() then LoggingCombat(true) end`; se dopo resta spento, stampa `"|cffffd100Diario di Gilda|r: il registro dei combattimenti è spento. Scrivi /combatlog per accenderlo."` e scrive un evento `note`.
  - `PLAYER_LOGOUT` → `rec:flush()`, `Journal.finish()`.
  - `CHAT_MSG_LOOT(msg)`, `LOOT_READY` (`IsFishingLoot()`, sorgente = `UnitName("target")` se `UnitIsDead("target")`), `UNIT_SPELLCAST_SUCCEEDED("player", _, spellId)`, `PLAYER_LEVEL_UP(level)`, `PLAYER_DEAD`, `MIRROR_TIMER_START("BREATH")`/`MIRROR_TIMER_STOP("BREATH")`, `PLAYER_MONEY` (`GetMoney()`, `where` da stato aperto/chiuso di `MERCHANT_SHOW/CLOSED`, `AUCTION_HOUSE_SHOW/CLOSED`, `TRAINER_SHOW/CLOSED`), `PLAYER_REGEN_DISABLED/ENABLED`, `UNIT_HEALTH("player")` (`UnitHealth`, `UnitHealthMax`), `ENCOUNTER_START/END`, `GROUP_ROSTER_UPDATE` (membri da `party1..4` o `raid1..40`, `UnitIsInMyGuild`), `QUEST_TURNED_IN(id, xp, money)` (titolo da `C_QuestLog.GetTitleForQuestID`), `hooksecurefunc("AbandonQuest", …)` (id da `C_QuestLog.GetAbandonQuest()`, titolo come sopra), `CHAT_MSG_SYSTEM(msg)`, `ZONE_CHANGED_NEW_AREA`.
  - `deps.where()`: `GetZoneText`, `GetSubZoneText`, `C_Map.GetBestMapForUnit("player")`, `C_Map.GetPlayerMapPosition(m, "player"):GetXY()` arrotondati a 3 decimali, `IsInInstance()` + `GetInstanceInfo()` per `inst`. Ogni chiamata passa da `Safe.call` e `Safe.get`.
  - `deps.itemInfo(link)`: `C_Item.GetItemInfo(link)` (nome, qualità) e `C_Item.GetItemInfoInstant(link)` (id).
  - `deps.after`: `C_Timer.After`.
  - `/diario` stampa: versione, personaggio, eventi nella sessione corrente, registro combattimenti acceso/spento, numero di sessioni salvate, numero di errori. `/diario debug` attiva/disattiva la stampa di ogni `UNIT_SPELLCAST_SUCCEEDED` del giocatore (`spellId` e nome), per verificare gli id della pesca al lancio.

- [ ] **Step 1:** In `helpers.lua` aggiungi `helpers.gameStub()`: un `CreateFrame` finto che registra gli handler, `helpers.fire(event, ...)`, e stub delle API sopra (valori modificabili dal test). Test in `main_spec.lua`: `ADDON_LOADED` + `PLAYER_ENTERING_WORLD(true,false)` → `DiarioGildaDB.sessions[1].char == "Gino"` e un evento `zone`; `PLAYER_LOGOUT` → `stop` valorizzato; `CHAT_MSG_LOOT` con oggetto raro → evento `loot`; `LoggingCombat` che resta false → evento `note` e messaggio stampato; un'API che lancia errore dentro un handler → nessuna eccezione verso il chiamante, `errors` ha una voce; `/diario` stampa una riga con `"eventi"`.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS, luacheck pulito.
- [ ] **Step 5:** Commit `feat(addon): collegamento agli eventi del gioco e /diario`.

### Task 8: Pacchetto e lista di verifica del lancio

**Files:** Create `scripts/package-addon.sh`, `docs/verifica-al-lancio.md`, `tests/fixtures/DiarioGilda.lua`.

**Interfaces:**
- Produces: `dist/DiarioGilda-<versione>.zip` con la cartella `DiarioGilda/` (solo TOC, `Core/`, `Main.lua`). Il Piano 2 incorpora la stessa cartella nel programmino.
- Produces: `tests/fixtures/DiarioGilda.lua`, un file SavedVariables d'esempio realistico (una sessione completa con la fuga dai Murloc: `fishing`, `combat` con `minHp=12`, un `loot` raro, un `encounter` riuscito, un `group`, più una sessione interrotta senza `stop`). Il Piano 2 usa questo file come fixture.

- [ ] **Step 1:** Genera la fixture con un test (`tests/lua/spec/fixture_spec.lua`) che guida il Recorder con eventi simulati, serializza `DiarioGildaDB` nel formato di WoW (`DiarioGildaDB = { ["chiave"] = valore, … }`, una funzione `helpers.serialize`) e confronta con il file salvato (se il file manca lo scrive, poi fallisce chiedendo di rieseguire).
- [ ] **Step 2:** Script del pacchetto; verifica: `unzip -l dist/DiarioGilda-0.1.0.zip` elenca TOC, Core e Main.lua, nient'altro.
- [ ] **Step 3:** `docs/verifica-al-lancio.md`: lista da spuntare in gioco il 5 novembre: nome della cartella del client e percorso di `SavedVariables`; `/diario` dopo login; `LoggingCombat(true)` accende davvero il registro (file in `Logs/`); id della pesca con `/diario debug`; un drop raro, una morte, un level up, un cambio zona, un gruppo e un boss finiscono nel file dopo `/reload`; `CHAT_MSG_LOOT` e `CHAT_MSG_SYSTEM` in dungeon (secret?); nessun errore Lua con `/console scriptErrors 1`. Per ogni voce: cosa fare se fallisce.
- [ ] **Step 4:** `bash scripts/test-lua.sh` → PASS.
- [ ] **Step 5:** Commit `feat(addon): pacchetto, fixture e verifica al lancio`.
