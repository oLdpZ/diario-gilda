# Diario di Gilda — Piano 2: Programmino (Windows e macOS) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un'app per Windows e macOS che installa e aggiorna da sola l'addon e sé stessa, e a ogni logout manda a Discord la sessione dell'addon arricchita con gli episodi di combattimento letti da `WoWCombatLog`.

**Architecture:** Un modulo Go in `app/` con pacchetti piccoli in `app/internal/`, ciascuno testato da solo; `app/internal/run` li compone in un ciclo (leggi SavedVariables → episodi → pacchetto → coda → webhook). L'addon del Piano 1 è incorporato nell'eseguibile con `go:embed`, così aggiornare l'app aggiorna anche l'addon. Gli aggiornamenti arrivano da GitHub Releases, firmati ed25519.

**Tech Stack:** Go 1.23+, `fyne.io/systray` (icona), `github.com/ncruces/zenity` (finestre di dialogo), `github.com/minio/selfupdate` (sostituzione dell'eseguibile), `golang.org/x/sys/windows/registry` (avvio automatico su Windows); stdlib per HTTP, gzip, JSON, ed25519.

**Spec:** `docs/superpowers/specs/2026-10-23-diario-di-gilda-design.md` · Contratto dati: `docs/formato-dati.md` (Piano 1). Il Piano 1 deve essere completato prima di Task 2.

## Global Constraints

- Piattaforme: Windows amd64 e macOS (universal arm64+amd64). Nessuna installazione richiesta oltre all'app.
- Repository degli aggiornamenti: `oLdpZ/diario-gilda` (pubblico, solo codice). Asset per release: `DiarioGilda-windows-amd64.zip`, `DiarioGilda-macos.zip`, ognuno con un file `.sig` (firma ed25519 del file zip, base64).
- Valori incorporati al build con `-ldflags -X` in `app/internal/build`: `Version`, `WebhookURL`, `PublicKey` (ed25519, base64). Il webhook non sta mai nel codice sorgente.
- Configurazione utente: `os.UserConfigDir()/DiarioGilda/config.json`; stato: `…/DiarioGilda/state.json`; coda: `…/DiarioGilda/outbox/`; log: `…/DiarioGilda/diario.log` (max 1 MB, ruotato).
- Ogni testo visibile al giocatore è in italiano.
- File inviato: gzip di JSON, sotto `MaxUpload = 8 * 1024 * 1024` byte (il limite di Discord senza boost è 10 MB).
- Il programmino non scrive mai nei file di WoW tranne la cartella `Interface/AddOns/DiarioGilda/`.
- Test: `cd app && go test ./... && go vet ./...`. Go va installato sul PC dell'autore se manca: `winget install GoLang.Go`.

## Review Focus

- WoW è installato in un percorso non standard o su un altro disco: la ricerca fallisce e l'app deve chiedere la cartella, non bloccarsi (test in Task 3).
- PC senza internet al logout: la sessione resta in coda e parte alla prossima occasione, senza duplicati (test in Task 7).
- Registro combattimenti spento o file assente: la sessione parte comunque, senza episodi (test in Task 8).
- Il file SavedVariables viene letto mentre WoW lo sta ancora scrivendo: l'app aspetta che sia stabile e, se il parse fallisce, riprova invece di scartare (test in Task 8).
- Un aggiornamento scaricato a metà o manomesso: va rifiutato e l'app resta alla versione precedente funzionante (test in Task 9).

---

### Task 1: Modulo Go, build info, configurazione

**Files:**
- Create: `app/go.mod` (`module github.com/oLdpZ/diario-gilda/app`), `app/internal/build/build.go`, `app/internal/config/config.go`, `app/internal/config/config_test.go`, `.github/workflows/ci.yml`

**Interfaces:**
- Produces: `build.Version`, `build.WebhookURL`, `build.PublicKey` (string, default `"dev"`, `""`, `""`).
- Produces: `type config.Config struct { Player string; WowDir string; ClientDir string; Webhook string }`; `config.Dir() (string, error)`; `config.Load(dir string) (Config, error)` (file assente → Config vuota, nessun errore; `Webhook` vuoto → `build.WebhookURL`); `config.Save(dir string, c Config) error` (scrittura atomica: file temporaneo + rename).

- [ ] **Step 1:** Test: `Load` su cartella vuota → zero value senza errore; `Save` poi `Load` → uguale; JSON corrotto → errore che contiene `config.json`; `Webhook` vuoto prende il default di build.
- [ ] **Step 2:** `cd app && go test ./internal/config/` → FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** `ci.yml`: su push e PR, job Ubuntu con `go test ./... && go vet ./...` in `app/` e un job Ubuntu per il Lua 5.1 (`luarocks install busted luacheck`, poi `busted` e `luacheck addon tests`).
- [ ] **Step 6:** Commit `feat(app): modulo, configurazione e CI`.

### Task 2: `internal/svparse` e `internal/addondata` — lettura delle SavedVariables

**Files:** Create `app/internal/svparse/parse.go`, `parse_test.go`, `app/internal/addondata/addondata.go`, `addondata_test.go`; la fixture è `tests/fixtures/DiarioGilda.lua` (Piano 1, Task 8).

**Interfaces:**
- Produces: `svparse.Parse(src []byte) (map[string]any, error)`: assegnazioni globali `Nome = valore` in sequenza; valori: tabelle (`["k"] = v`, `[n] = v`, elementi posizionali), stringhe con escape Lua (`\"`, `\\`, `\n`, `\ddd`), numeri (interi e decimali, negativi), `true`/`false`/`nil`, virgole finali, commenti `--`. Le tabelle con sole chiavi intere consecutive da 1 diventano `[]any`, le altre `map[string]any` (chiavi intere come stringhe decimali).
- Produces: `addondata.Decode(globals map[string]any) (DB, error)`, con `type DB struct { V int; Sessions []Session }`, `type Session struct { ID, Char, Realm, Guild, Class, Race, Build, Addon string; Level int; Start int64; Stop *int64; Dropped int; Events []Event }`, `type Event struct { T int64; K string; Fields map[string]any }` (`Fields` = tutti i campi tranne `t` e `k`). `V != 1` → errore `formato non supportato`. Serializzazione JSON di `Session`: chiavi come nel contratto dati (`id`, `char`, …, `events` con `t`, `k` e i campi appiattiti).

- [ ] **Step 1:** Test di `svparse`: casi unitari per ogni tipo, stringa con `]]` e `\"`, array vs mappa, errore con numero di riga su input troncato. Test di `addondata` sulla fixture: 2 sessioni, la prima con `Stop != nil`, un evento `combat` con `Fields["minHp"] == int64(12)` (regola di `svparse`: numero senza parte decimale → `int64`, altrimenti `float64`), la seconda con `Stop == nil`; JSON di una `Session` riletto ha `events[0].t` e `events[0].k`.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa (parser ricorsivo a mano, nessuna dipendenza). **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(app): lettura delle SavedVariables dell'addon`.

### Task 3: `internal/wowdir` — trovare WoW, il client Forever e i file

**Files:** Create `app/internal/wowdir/wowdir.go`, `wowdir_test.go`.

**Interfaces:**
- Produces: `wowdir.DefaultRoots(goos string) []string` (Windows: `C:\Program Files (x86)\World of Warcraft`, `C:\Program Files\World of Warcraft`, più `D:\…` ed `E:\…` con gli stessi due nomi; macOS: `/Applications/World of Warcraft`).
- Produces: `wowdir.Clients(root string) []string`: sottocartelle di `root` che contengono sia `Interface` sia `WTF`, ordinate: prima i nomi che contengono `forever`, poi `_classic_beta_`, poi le altre.
- Produces: `wowdir.SavedVariables(clientDir string) []string`: tutti i `WTF/Account/*/SavedVariables/DiarioGilda.lua` esistenti.
- Produces: `wowdir.AddOnsDir(clientDir string) string`, `wowdir.LogsDir(clientDir string) string`.

- [ ] **Step 1:** Test con alberi in `t.TempDir()`: radice con `_retail_` e `_forever_` → `_forever_` primo; radice senza client → slice vuota; due account con il file → due percorsi; cartella con solo `Interface` esclusa.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(app): ricerca della cartella di WoW Forever`.

### Task 4: `internal/combatlog` — lettura di `WoWCombatLog`

**Files:** Create `app/internal/combatlog/combatlog.go`, `combatlog_test.go`, `app/internal/combatlog/testdata/murloc.txt`.

**Interfaces:**
- Produces: `type Event struct { Time time.Time; Sub string; SrcGUID, SrcName string; SrcFlags uint32; DstGUID, DstName string; DstFlags uint32; SpellName string; Amount int64; Crit bool; EnvType string }`.
- Produces: `combatlog.ParseLine(line string, loc *time.Location, year int) (Event, bool)`. Formato: `M/D[/YYYY] H:MM:SS.fff[±tz]  SUB,campi…` (due spazi tra data/ora e CSV; i campi tra virgolette possono contenere virgole). Prefisso base dopo `SUB`: `srcGUID, srcName, srcFlags, srcRaidFlags, dstGUID, dstName, dstFlags, dstRaidFlags` (flag in esadecimale `0x…`). Per `SPELL_*`, `RANGE_*`: poi `spellId, spellName, spellSchool`; per `ENVIRONMENTAL_*`: poi `envType`. Se il campo successivo ha forma di GUID (`Player-`, `Creature-`, `Pet-`, `0000000000000000`) la riga ha i 17 campi del registro avanzato, da saltare. Poi il suffisso: per `*_DAMAGE` `amount` è il primo campo e `critical` il settimo dopo `amount` (`"1"`/`"nil"`); per `*_HEAL` `amount` primo, `critical` quinto. Sottoeventi non riconosciuti → `ok = true` con solo i campi base. Riga malformata → `ok = false`.
- Produces: `combatlog.Files(logsDir string) []string` (`WoWCombatLog*.txt`), `combatlog.Read(paths []string, from, to time.Time, loc *time.Location) ([]Event, error)`: legge in streaming, tiene solo gli eventi in `[from, to]`, salta i file con data di modifica precedente a `from`.

- [ ] **Step 1:** Crea `testdata/murloc.txt` (circa 40 righe nel formato del registro moderno: 6 `Creature-…` diversi chiamati `Murloc Forager`/`Murloc Netter` che fanno `SWING_DAMAGE` su `Player-…` `"Gino-Realm"` con flag ostili `0xa48`, cure `SPELL_HEAL` da `"Mara-Realm"`, un `ENVIRONMENTAL_DAMAGE` `Falling`, un `UNIT_DIED`, righe con e senza campi avanzati). Test: `ParseLine` di una `SWING_DAMAGE` con e senza campi avanzati → stesso `Amount`; nome con virgola tra virgolette; `ENVIRONMENTAL_DAMAGE` → `EnvType == "Falling"`; riga vuota → `ok == false`; `Read` con finestra che esclude le prime righe → conteggio corretto.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Annota in `docs/verifica-al-lancio.md` (Piano 1): catturare un `WoWCombatLog` vero di Forever e aggiungerlo a `testdata/` per confermare posizioni dei campi.
- [ ] **Step 6:** Commit `feat(app): lettura del registro dei combattimenti`.

### Task 5: `internal/episodes` — riassunto dei combattimenti

**Files:** Create `app/internal/episodes/episodes.go`, `episodes_test.go`.

**Interfaces:**
- Consumes: `addondata.Session`, `combatlog.Event`.
- Produces: `type Episode struct { From, To int64; Kind string /* "combat" | "death" */; MinHP *int; EnemyCount int; Enemies []NameCount; DamageTaken int64; BiggestHitTaken *Hit; BiggestHitDone *Hit; KilledBy *Hit; Healers []NameAmount; Environmental []NameAmount; Kills []NameCount }`, `type Hit struct { Who, Spell string; Amount int64; Crit bool }`, `type NameCount struct { Name string; N int }`, `type NameAmount struct { Name string; Amount int64 }` (tag JSON in camelCase).
- Produces: `episodes.Build(s addondata.Session, evs []combatlog.Event, loc *time.Location) []Episode`. Il giocatore è `s.Char + "-" + s.Realm` (confronto anche solo su `s.Char` se il nome nel log non ha reame). Un episodio `combat` per ogni evento `combat` della sessione su `[from-2, to+2]`, con `MinHP` dal campo `minHp`; un episodio `death` per ogni `death` su `[t-15, t]`. Nemici: GUID distinti con flag ostile (`0x40`) che hanno fatto danno al giocatore, raggruppati per nome; `Kills`: `UNIT_DIED` di GUID a cui il giocatore ha fatto danno; `Healers`: cure ricevute da altri, ordinate per quantità, max 5; `KilledBy`: l'ultimo colpo subito prima di `UNIT_DIED` del giocatore. Liste ordinate in modo deterministico.

- [ ] **Step 1:** Test con `testdata/murloc.txt` di Task 4 e una sessione costruita a mano (evento `combat` con `minHp` 12): `EnemyCount == 6`, `Enemies` contiene `Murloc Forager` con N corretto, `MinHP == 12`, `Healers[0].Name == "Mara-Realm"`; sessione con evento `death` e caduta → episodio `death` con `Environmental[0].Name == "Falling"`; nessun evento del log → episodi con soli `From/To/MinHP`; evento estraneo di un altro giocatore → ignorato.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(app): episodi di combattimento`.

### Task 6: `internal/payload` — il file da inviare

**Files:** Create `app/internal/payload/payload.go`, `payload_test.go`.

**Interfaces:**
- Produces: `payload.Build(player, appVersion string, s addondata.Session, eps []episodes.Episode, interrupted bool) (name string, data []byte, err error)`. JSON: `{"formato":1,"giocatore":…,"app":…,"interrotta":bool,"sessione":<Session>,"combattimenti":[<Episode>…]}`, compresso gzip. Nome: `sessione_<player>_<s.ID>.json.gz` con caratteri non `[A-Za-z0-9_-]` sostituiti da `_`. Se compresso supera `MaxUpload`: prima toglie `Enemies`, `Healers`, `Kills` dagli episodi, poi tronca gli eventi più vecchi aggiungendo `"troncata": n`.
- Produces: `payload.MaxUpload = 8 << 20`.

- [ ] **Step 1:** Test: decomprimi e verifica `formato == 1`, `giocatore`, numero di eventi; nome con spazi e accenti sanificato; sessione enorme generata (200k eventi) → risultato sotto `MaxUpload` con `troncata > 0`.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(app): pacchetto della sessione`.

### Task 7: `internal/discord`, `internal/outbox`, `internal/state` — invio affidabile

**Files:** Create `app/internal/discord/webhook.go`, `webhook_test.go`, `app/internal/outbox/outbox.go`, `outbox_test.go`, `app/internal/state/state.go`, `state_test.go`.

**Interfaces:**
- Produces: `discord.Upload(ctx context.Context, webhookURL, filename string, data []byte, content string) error`: POST multipart con `payload_json` (`{"content": content}`) e `files[0]`; 2xx → nil; 429 → errore `*discord.RetryAfter{Seconds float64}` letto dal corpo JSON `retry_after`; altri codici → errore con codice e corpo (troncato a 200 caratteri).
- Produces: `outbox.Put(dir, name string, data []byte) error` (scrittura atomica), `outbox.Flush(ctx, dir string, send func(name string, data []byte) error) (sent int, err error)`: invia in ordine di nome, cancella ciò che è partito, si ferma al primo errore lasciando il resto.
- Produces: `type state.State struct { Sent map[string]int64 /* id sessione → epoch invio */; LastUpdateCheck int64 }`, `state.Load(dir)`, `state.Save(dir, st)` (atomico), `st.MarkQueued(id string, now int64)`, `st.Has(id string) bool`. Voci più vecchie di 60 giorni potate al `Load`.

- [ ] **Step 1:** Test con `httptest.Server`: upload valido arriva con il file e il nome giusti; risposta 429 con `{"retry_after": 1.5}` → `RetryAfter{1.5}`; 500 → errore. Outbox: tre file, `send` che fallisce al secondo → `sent == 1`, restano due file; nuovo `Flush` con `send` ok → cartella vuota. State: `MarkQueued`, `Save`, `Load` → `Has` vero; voce di 61 giorni fa potata.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(app): invio a Discord con coda e stato`.

### Task 8: `internal/run` — il ciclo completo e la sorveglianza dei file

**Files:** Create `app/internal/run/run.go`, `run_test.go`.

**Interfaces:**
- Consumes: Task 1–7.
- Produces: `type run.Env struct { Cfg config.Config; DataDir string; Now func() time.Time; Loc *time.Location; Send func(name string, data []byte) error; Log func(format string, args ...any) }`.
- Produces: `run.Cycle(ctx, env Env) error`: per ogni file di `wowdir.SavedVariables(cfg.ClientDir)` → `svparse` + `addondata`; per ogni sessione non ancora in `state` che ha `Stop != nil`, oppure `Stop == nil` e ultimo evento più vecchio di 12 ore (`interrotta = true`): episodi da `combatlog.Read` su `[Start-60s, Stop+60s]`, `payload.Build`, `outbox.Put`, `st.MarkQueued`; salva lo stato; poi `outbox.Flush` con `env.Send`. Errore di parse di un file → log e nessuna sessione marcata (si riprova al giro dopo). Registro assente → episodi senza dati del log.
- Produces: `run.Watch(ctx, env Env, poll time.Duration, stable time.Duration)`: ogni `poll` (10 s) controlla `ModTime` e dimensione dei file SavedVariables; quando cambiano e restano uguali per `stable` (5 s) chiama `Cycle`; chiama comunque `Cycle` ogni 5 minuti (per la coda) e all'avvio.

- [ ] **Step 1:** Test end-to-end in `t.TempDir()`: albero WoW finto con la fixture del Piano 1 e `testdata/murloc.txt` copiato in `Logs/`; `Send` finto che registra → primo `Cycle` invia 1 file (la sessione chiusa; quella aperta è recente se `Now` è vicino), secondo `Cycle` non invia nulla; con `Now` + 13 ore la sessione aperta parte con `interrotta: true`; `Send` che fallisce → file resta nell'outbox, lo stato la segna comunque in coda (nessun duplicato al giro dopo), al giro successivo con `Send` ok parte; SavedVariables troncato a metà → nessun invio, nessuna sessione marcata, al giro dopo con il file intero parte; `Logs/` vuota → invio con `combattimenti` senza dati del log.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(app): ciclo di raccolta e invio`.

### Task 9: `internal/addoninstall` e `internal/update` — addon incorporato e aggiornamento automatico

**Files:** Create `app/internal/addoninstall/embed.go` (`//go:embed all:DiarioGilda`; la cartella `app/internal/addoninstall/DiarioGilda/` è una copia di `addon/DiarioGilda` fatta da `scripts/build-app.sh` e dal workflow di release, ignorata da git; per i test `go generate` la copia), `app/internal/addoninstall/install.go`, `install_test.go`, `app/internal/update/update.go`, `update_test.go`, `app/cmd/firma/main.go`.

**Interfaces:**
- Produces: `addoninstall.Embedded fs.FS` (radice `DiarioGilda/`), `addoninstall.Version(fsys fs.FS) string` (riga `## Version:` del TOC), `addoninstall.Installed(addonsDir string) string` (versione installata o `""`), `addoninstall.Install(fsys fs.FS, addonsDir string) error`: copia in `addonsDir/DiarioGilda.new`, poi rinomina l'attuale in `DiarioGilda.old`, la nuova in `DiarioGilda`, cancella `.old`; in caso di errore ripristina `.old`. `addoninstall.NeedsInstall(fsys, addonsDir) bool` (assente o versione diversa).
- Produces: `update.Check(ctx, client *http.Client, apiBase, repo, current string) (*Release, error)`: `GET {apiBase}/repos/{repo}/releases/latest`; `Release{Version string; AssetURL, SigURL string}` per l'asset di `runtime.GOOS` (nomi da Global Constraints); nil se `Version` (senza `v`) non è più nuovo di `current` (confronto semver numerico); `current == "dev"` → mai aggiornare.
- Produces: `update.Verify(data, sig []byte, pubKeyB64 string) error`, `update.Apply(zipData []byte) error`: estrae l'eseguibile dallo zip (Windows: `DiarioGilda.exe`; macOS: `DiarioGilda.app/Contents/MacOS/DiarioGilda`) e lo applica con `selfupdate.Apply` (con `OldSavePath` per il ripristino); su macOS sostituisce anche le altre risorse del bundle se presenti.
- Produces: `cmd/firma`: `firma keygen` stampa chiave pubblica e privata base64; `firma sign <file>` legge la privata da `FIRMA_KEY` e scrive `<file>.sig`.

- [ ] **Step 1:** Test: `Install` su cartella vuota → file presenti e `Installed == Version(Embedded)`; seconda installazione con versione diversa → sostituita, nessun `.old` rimasto; `Install` con `addonsDir` di sola lettura → errore e versione precedente intatta; `NeedsInstall` vero/falso. Update con `httptest`: release `v0.2.0` con `current = "0.1.0"` → Release con l'URL dell'asset giusto per il GOOS; `v0.1.0` → nil; `current = "dev"` → nil; `Verify` con firma giusta → nil, con un byte cambiato → errore; `firma keygen`/`sign` usati nel test per produrre la coppia.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(app): addon incorporato e aggiornamento firmato`.

### Task 10: L'app sul desktop — primo avvio, icona, avvio automatico

**Files:** Create `app/cmd/diario/main.go`, `app/internal/desktop/firstrun.go`, `app/internal/desktop/tray.go`, `app/internal/autostart/autostart_windows.go`, `autostart_darwin.go`, `autostart_test.go`, `app/internal/desktop/icon.png`, `app/internal/desktop/lock.go`.

**Interfaces:**
- Consumes: tutto il resto.
- Produces: `autostart.Enable(exePath string) error`, `autostart.Enabled() bool`. Windows: valore `DiarioGilda` in `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` = `"<exePath>"`. macOS: `~/Library/LaunchAgents/it.oldpz.diariogilda.plist` con `RunAtLoad` true e `ProgramArguments` = percorso dell'eseguibile nel bundle; funzione pura `autostart.Plist(exePath string) []byte` testata.
- Produces: comportamento di `main`:
  1. Un solo processo alla volta (`lock.go`: file di lock in `config.Dir()`); un secondo avvio esce subito.
  2. Primo avvio (config senza `Player` o `ClientDir`): `wowdir.DefaultRoots` + `Clients`; se nessun client trovato, `zenity.SelectFile(zenity.Directory())` con titolo `"Dov'è installato World of Warcraft?"` e ricerca dentro la cartella scelta; poi `zenity.Entry("Con che nome vuoi comparire nel diario?")`; salva config; `autostart.Enable`; messaggio `"Fatto! Il Diario di Gilda ora lavora da solo. Lo trovi vicino all'orologio."`.
  3. A ogni avvio: se `NeedsInstall`, installa l'addon e, se WoW è aperto, mostra la notifica `"Addon aggiornato: scrivi /reload in gioco."`.
  4. `run.Watch` in background; controllo aggiornamenti all'avvio e ogni 6 ore (`update.Check` → `Verify` → `Apply` → riavvio del processo).
  5. Menu dell'icona: `"Diario di Gilda <versione>"` (disabilitata), `"Ultimo invio: <ora>"` o `"Nessun invio ancora"`, `"Invia ora"` (`run.Cycle`), `"Cambia nome…"`, `"Apri cartella dei registri"`, `"Esci"`.

- [ ] **Step 1:** Test di `autostart.Plist`: XML valido con `it.oldpz.diariogilda`, `RunAtLoad` e il percorso passato; test del lock: secondo `Acquire` nella stessa cartella fallisce, dopo `Release` riesce.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS; `GOOS=windows go build ./cmd/diario` e `GOOS=darwin go vet ./...` senza errori.
- [ ] **Step 5:** Prova a mano su Windows con un webhook di test: primo avvio, nome, addon installato in una cartella WoW finta, icona e menu, "Invia ora" fa arrivare il file sul canale di test. Annota l'esito nel messaggio di commit.
- [ ] **Step 6:** Commit `feat(app): primo avvio, icona e avvio automatico`.

### Task 11: Release e guide

**Files:** Create `.github/workflows/release.yml`, `app/packaging/macos/Info.plist`, `scripts/build-app.sh`, `docs/guida-installazione.md`, `docs/guida-discord.md`.

**Interfaces:**
- Produces: al push di un tag `v*`: copia `addon/DiarioGilda` in `app/internal/addoninstall/DiarioGilda` (con `## Version` = tag senza `v`); build Windows su `ubuntu-latest` (`GOOS=windows GOARCH=amd64`, `-ldflags "-H windowsgui -X …build.Version=… -X …build.WebhookURL=${{ secrets.WEBHOOK_URL }} -X …build.PublicKey=${{ vars.PUBLIC_KEY }}"`) → `DiarioGilda.exe` in `DiarioGilda-windows-amd64.zip`; build macOS su `macos-14` per arm64 e amd64 con cgo, `lipo -create`, bundle `DiarioGilda.app` con `Info.plist` (`LSUIElement` true, `CFBundleIdentifier` `it.oldpz.diariogilda`, `CFBundleShortVersionString` = versione) → `DiarioGilda-macos.zip`; `go run ./cmd/firma sign` su ogni zip con `secrets.FIRMA_KEY`; release GitHub con zip e `.sig`.
- Produces: `docs/guida-installazione.md` (per i giocatori, italiano, due minuti): scaricare lo zip giusto; Windows: estrarre in `Documenti\DiarioGilda`, doppio clic, "Ulteriori informazioni → Esegui comunque" se compare SmartScreen; macOS: spostare l'app in Applicazioni, tasto destro → Apri la prima volta; scegliere il nome; in gioco `/diario` per controllare; cosa viene raccolto (eventi di gioco con nome, posizione e orari; mai la chat) e cosa no; riserva manuale (trascinare `DiarioGilda.lua` da `WTF/Account/<ACCOUNT>/SavedVariables/` nel canale `#diario-servizio`).
- Produces: `docs/guida-discord.md` (per l'autore): creare `#diario-gilda` e `#diario-servizio` (visibile, notifiche silenziate), creare il webhook su `#diario-servizio`, creare l'applicazione bot con permessi `View Channel`, `Read Message History`, `Send Messages`, `Embed Links`, `Attach Files` solo su quei due canali; impostare i segreti del repository `WEBHOOK_URL`, `FIRMA_KEY` e la variabile `PUBLIC_KEY` (da `firma keygen`); creare il repository pubblico `oLdpZ/diario-gilda`.

- [ ] **Step 1:** `scripts/build-app.sh` fa in locale la build Windows con gli stessi flag (webhook da variabile d'ambiente); verifica: `bash scripts/build-app.sh` produce `dist/DiarioGilda-windows-amd64.zip` e `go run ./cmd/firma sign` ne produce il `.sig`.
- [ ] **Step 2:** Scrivi workflow, `Info.plist` e le due guide.
- [ ] **Step 3:** Prova del ciclo di aggiornamento: tag `v0.1.0-test` su un fork o con `apiBase` puntato a un server locale, poi `v0.1.1-test`; l'app 0.1.0 si aggiorna e reinstalla l'addon. Annota l'esito.
- [ ] **Step 4:** Commit `feat(app): release firmate e guide`.
