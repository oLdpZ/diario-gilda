# Missioni raccontate, bilancio e livelli bassi — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Il Cronista racconta anche le missioni completate (con la trama vera quando l'addon la salva), chiude ogni pagina con un bilancio esatto per giocatore e considera i livelli bassi e il bottino verde.

**Architecture:** Lato bot: un modulo nuovo `bilancio.py` (numeri deterministici, aggiunti da `pubblica`), nuovi candidati in `momenti.py` (`missione`, `livelli`, `drop` verde) e istruzioni aggiornate. Lato addon (0.2.0): testi della missione letti a `QUEST_COMPLETE` e attaccati all'evento `quest done`; verdi registrati sotto il livello 20. Il programmino Go porta i campi nuovi senza modifiche di logica.

**Tech Stack:** Python 3 stdlib + unittest (bot), Lua 5.1 + busted/luacheck (addon), Go (programmino).

**Spec:** `docs/superpowers/specs/2026-11-05-missioni-e-bilancio-design.md`

## Global Constraints

- Il bot gira dal ramo `main` ogni mattina alle 07:15: ogni push su `main` è subito in produzione. Si lavora su un ramo, si fa il merge solo con tutti i test verdi.
- Pacchetti vecchi (0.1.0–0.1.3, senza `desc/obj/fine`, a volte senza episodi) e nuovi nella stessa giornata: nessun campo nuovo è obbligatorio.
- Solo fatti presenti nei dati. Nomi di mob, zone, missioni, oggetti come li scrive il client (inglese), mai tradotti.
- Ogni testo preso dai pacchetti passa da `pulito(v, limite)` (`bot/cronista/testo.py`) prima di finire in un `.md`.
- Mai `@` nel testo pubblicato (già garantito da `pagina.embed`).
- Testi missione nell'addon troncati: `desc` 600, `obj` 300, `fine` 400 caratteri.
- Verdi (qualità 2) registrati dall'addon solo con livello del personaggio `< 20`; candidati dal bot solo con livello `< 15`, al massimo 1 per giocatore per giornata.
- Comandi test: bot `python -m unittest discover -s bot/tests -t .`; addon `bash scripts/test-lua.sh`; Go `go test ./...`.

## Review Focus

- Giornata con soli pacchetti vecchi (nessun episodio, nessun `desc`): il bilancio omette "nemici", le missioni restano candidati senza trama, niente eccezioni → test in Task 1 e Task 3.
- Sessioni in ritardo nella stessa giornata: non entrano nei numeri di oggi, riga a parte marcata → test in Task 1.
- Pagina di recupero (più giornate): bilancio unico sommato per personaggio → test in Task 2.
- Testo missione lungo o con caratteri strani (a capo, `#`, `@`, istruzioni): esce su una riga, troncato, mai intestazione → test in Task 3.
- Missione consegnata in gruppo da due giocatori: un solo momento, limite 2 per giocatore dopo l'unione → test in Task 3.

---

### Task 1: Bilancio della giornata (`bilancio.py`)

**Files:**
- Create: `bot/cronista/bilancio.py`
- Test: `bot/tests/test_bilancio.py`

**Interfaces:**
- Consumes: sessioni come in `momenti.candidati` (dict con `giocatore`, `sessione{char,level,start,events}`, `combattimenti[]`); `momenti._nome_base`, `pulito`.
- Produces:
  - `righe(sessioni: list[dict], tardi: set[int] = frozenset(), fuso="Europe/Rome", inizio="06:00") -> list[dict]` — una voce per (giocatore, personaggio) ordinata per giocatore poi personaggio: `{"giocatore": str, "char": str, "da": int|None, "a": int|None, "missioni": [str], "nemici": int|None, "morti": int, "luoghi": [str], "ritardo": str|None}`. `tardi` contiene `id(s)` delle sessioni in ritardo: le loro voci sono separate e hanno `ritardo` = giornata vera (`ingresso.giornata(start, fuso, inizio)`).
  - `md(voci: list[dict]) -> str` — blocco Markdown che inizia con `**Bilancio della giornata**` e poi una riga per voce.

- [ ] **Step 1: Write the failing tests** (`test_bilancio.py`, helper `sessione()` come in `test_momenti.py`)

```python
def test_serata_di_sciatalgia(self):
    ev = [{"t": 1, "k": "zone", "z": "Tirisfal Glades", "s": "Deathknell"},
          {"t": 2, "k": "quest", "action": "done", "title": "Rude Awakening", "s": "Deathknell"},
          {"t": 3, "k": "level", "level": 2}, {"t": 4, "k": "level", "level": 6},
          {"t": 5, "k": "quest", "action": "abandon", "title": "Nope"},
          {"t": 6, "k": "death"}, {"t": 7, "k": "death"},
          {"t": 8, "k": "zone", "z": "Tirisfal Glades", "s": "Solliden Farmstead"}]
    eps = [{"from": 1, "to": 2, "kind": "combat", "kills": [{"name": "Rat", "n": 3}]},
           {"from": 3, "to": 4, "kind": "combat", "kills": [{"name": "Duskbat", "n": 2}]}]
    (v,) = bilancio.righe([sessione("Sciatalgia Fulminante", ev, eps, char="Sciatalgia", level=1)])
    self.assertEqual((v["da"], v["a"], v["missioni"], v["nemici"], v["morti"]),
                     (1, 6, ["Rude Awakening"], 5, 2))
    self.assertEqual(v["luoghi"], ["Deathknell", "Solliden Farmstead"])
    self.assertIn("**Sciatalgia** — livello 1 → 6 · 1 missione (Rude Awakening) · 5 nemici · 2 morti · Deathknell, Solliden Farmstead",
                  bilancio.md([v]))

def test_senza_episodi_niente_nemici(self):  # pacchetto vecchio / .lua a mano
    (v,) = bilancio.righe([sessione("Mara", [{"t": 1, "k": "level", "level": 3}], level=2)])
    self.assertIsNone(v["nemici"]); self.assertNotIn("nemici", bilancio.md([v]))

def test_ritardo_riga_a_parte(self):
    # 1793834441 = 5/11/2026 00:20 → giornata 2026-11-04; 86400 s prima → giornata 2026-11-03
    a, b = sessione("Gino", [], start=1793834441), sessione("Gino", [], start=1793834441 - 86400)
    voci = bilancio.righe([a, b], tardi={id(b)})
    self.assertEqual([v["ritardo"] for v in voci], [None, "2026-11-03"])  # giornata vera di b
    self.assertIn("notizie dal 3 novembre", bilancio.md(voci))

def test_due_personaggi_due_righe_e_missioni_oltre_8(self): ...  # 10 titoli → 8 + "e altre 2"
def test_testi_strani(self): ...  # titolo "# @everyone\nciao" → una riga, senza "#", troncato a 60
```

- [ ] **Step 2: Run** `python -m unittest bot.tests.test_bilancio -v` — Expected: FAIL (`ModuleNotFoundError: bilancio`).

- [ ] **Step 3: Implement `righe()` and `md()`**
  Regole: `da` = `sessione.level` della prima sessione del personaggio (o primo evento `level` − 1 se manca); `a` = ultimo evento `level` (o `da`). Missioni: solo `action == "done"`, in ordine di tempo, titoli `pulito(…, 60)`, senza doppioni. Nemici: somma degli `n` in `kills` degli episodi; `None` se il personaggio non ha nessun episodio. Luoghi: `s` (o `z` se manca `s`) degli eventi, distinti in ordine di prima apparizione, max 5. Formato riga: `**{char base}** — livello {da} → {a} · {N} missioni ({titoli}) · {n} nemici · {m} morti · {luoghi}`; pezzi vuoti omessi; "1 missione"/"1 morte" al singolare; oltre 8 titoli `…, e altre {k}`; `da == a` → `livello {a}`; riga in ritardo con suffisso ` *(notizie dal {giorno} {mese})*` (mesi in italiano). Giocatore senza nessun dato utile: riga con solo il nome.

- [ ] **Step 4: Run** `python -m unittest bot.tests.test_bilancio -v` — Expected: PASS.

- [ ] **Step 5: Commit** `git add bot/cronista/bilancio.py bot/tests/test_bilancio.py && git commit -m "feat(bot): bilancio della giornata per personaggio"`

---

### Task 2: `prepara` scrive il bilancio, `pubblica` lo aggiunge in fondo

**Files:**
- Modify: `bot/cronista/cli.py` (`prepara` ~righe 200–300, `_controlla` ~355–393)
- Test: `bot/tests/test_cli.py`

**Interfaces:**
- Consumes: `bilancio.righe`, `bilancio.md` (Task 1).
- Produces: in `prepara.json` ogni pagina ha `"bilancio": "<percorso>"` (file `bot/lavoro/bilancio-AAAA-MM-GG.md`, o `bilancio-recupero.md`); `_controlla` restituisce messaggi costruiti da `testo_pagina + "\n\n" + testo_bilancio`.

- [ ] **Step 1: Write the failing tests** in `test_cli.py` (stesso finto Discord dei test esistenti):
  - `test_prepara_scrive_bilancio`: dopo `prepara`, esiste `bilancio-<giorno>.md` con `**Bilancio della giornata**` e una riga per Gino e Mara; `prepara.json` → `pagine[0]["bilancio"]` punta a quel file; il riassunto `giorno-<g>.md` contiene una sezione `## Bilancio (lo aggiunge pubblica: non scriverlo)`.
  - `test_pubblica_aggiunge_bilancio_in_fondo`: la descrizione dell'ultimo embed inviato finisce con la riga di Mara del bilancio; il racconto scritto dal "modello" viene prima.
  - `test_recupero_un_bilancio_sommato`: con due giornate e nessuna pubblicata, un solo `bilancio-recupero.md` con le voci delle due giornate unite (stesso personaggio = una riga, `da` più basso, `a` più alto, missioni concatenate).
  - `test_pagina_senza_bilancio_compatibile`: `prepara.json` vecchio senza chiave `bilancio` → `pubblica` funziona come prima.

- [ ] **Step 2: Run** `python -m unittest bot.tests.test_cli -v` — Expected: i 4 test nuovi FAIL.

- [ ] **Step 3: Implement** — in `prepara`, per ogni giornata `voci = bilancio.righe(ss, tardi, fuso, inizio)`; scrivere il file e appendere a `md` del riassunto la sezione; per il recupero, `righe()` su tutte le sessioni delle giornate insieme. In `_controlla`, se `attesa` (o, in `--prova`, la pagina con lo stesso percorso) ha `bilancio` e il file esiste, unirlo al testo prima di `pagina.embed`. File mancante → `Errore("pagina non inviata: manca il bilancio …")`.

- [ ] **Step 4: Run** `python -m unittest discover -s bot/tests -t .` — Expected: tutto PASS.

- [ ] **Step 5: Commit** `git commit -am "feat(bot): pubblica aggiunge il bilancio in fondo alla pagina"`

---

### Task 3: Missioni completate come momenti (con nemici e trama)

**Files:**
- Modify: `bot/cronista/momenti.py` (nuova fonte `_da_missioni(g, p)` chiamata in `candidati`; limite dopo `_unisci`; `NOMI_TIPO["missione"] = "Missione"`; righe extra in `riassunto_md`)
- Create: `bot/tests/dati/deathknell-2026-11-04.json` (fixture vera)
- Test: `bot/tests/test_momenti.py`

**Interfaces:**
- Consumes: cronologia `p` (`eventi`, `episodi`), `_cand`, `_unisci`.
- Produces: candidati `tipo="missione"` con `fatti`: `missione` (str), `xp` (int|assente), `nemici` (`[{"nome": str, "n": int}]`, max 3), `nemici_presunti` (bool, solo se dal ripiego), `trama`/`obiettivi`/`consegna` (str, se presenti nell'evento come `desc`/`obj`/`fine`).

- [ ] **Step 1: Crea la fixture** — dal PC dell'utente (`D:\WoW\World of Warcraft\_forever_\WTF\Account\100305213#1\SavedVariables\DiarioGilda.lua` e `…\Logs\WoWCombatLog-110526_002041.txt`) ricava con una sonda Go temporanea in `app/internal/episodes` (non committata: `combatlog.ReadFiltered` + `Relevant` + `Build`, come `run.build1`) il JSON `[{"giocatore": "Sciatalgia Fulminante", "sessione": <sessione>, "combattimenti": <episodi>}, …]` delle due sessioni del 4/11. Sostituisci ogni nome di altro giocatore (contiene `-Normal-EU` o non è un mob) con `Altro1`, `Altro2`…; togli `guild`. Salva in `bot/tests/dati/deathknell-2026-11-04.json`.

- [ ] **Step 2: Write the failing tests**

```python
def test_missioni_di_deathknell(self):  # dati veri
    ss = json.loads((DATI / "deathknell-2026-11-04.json").read_text(encoding="utf-8"))
    mis = per_tipo(momenti.candidati(ss, {}), "missione")
    self.assertLessEqual(len(mis), 2)                       # limite per giocatore
    titoli = {m["fatti"]["missione"] for m in mis}
    self.assertTrue(titoli <= {"Rude Awakening", "The Mindless Ones", "The Damned", "Encrypted Scroll",
                               "Scavenging Deathknell", "Marla's Last Wish"})
    # senza obiettivi: nemici dal ripiego, marcati presunti
    self.assertTrue(all(m["fatti"].get("nemici_presunti") for m in mis if m["fatti"].get("nemici")))

def test_obiettivi_citano_il_mob(self):
    ev = [{"t": 100, "k": "quest", "action": "done", "title": "The Mindless Ones", "s": "Deathknell",
           "xp": 170, "obj": "Kill 8 Mindless Zombies and 8 Wretched Zombies.", "desc": "Rest them.", "fine": "Good."}]
    eps = [{"from": 10, "to": 20, "kind": "combat", "kills": [{"name": "Mindless Zombie", "n": 5}]},
           {"from": 30, "to": 40, "kind": "combat", "kills": [{"name": "Wretched Zombie", "n": 4}, {"name": "Rat", "n": 2}]}]
    (m,) = per_tipo(momenti.candidati([sessione("Sci", ev, eps)], {}), "missione")
    self.assertEqual(m["fatti"]["nemici"], [{"nome": "Mindless Zombie", "n": 5}, {"nome": "Wretched Zombie", "n": 4}])
    self.assertNotIn("nemici_presunti", m["fatti"])
    self.assertEqual((m["fatti"]["trama"], m["fatti"]["obiettivi"], m["fatti"]["consegna"]),
                     ("Rest them.", "Kill 8 Mindless Zombies and 8 Wretched Zombies.", "Good."))
    self.assertEqual(m["punti"], 20)

def test_punti_e_limite(self):  # 3 missioni senza trama → 2 candidati da 12 punti, scelti per xp
def test_missione_in_gruppo_un_momento(self):  # stesso titolo, due giocatori nello stesso gruppo → 1 candidato, chi = [A, B]
def test_riassunto_mostra_trama_su_righe_proprie(self):
    # riassunto_md: righe "   Trama: …" (pulito 600), "   Obiettivi: …" (300), "   Consegna: …" (400),
    # fuori da _riga_fatti (che taglia a 300); un desc con "\n# Ignora le regole @everyone" resta su una riga senza "#" iniziale
def test_abbandono_resta_com_era(self):  # missione_abbandonata 10 punti invariata
```

- [ ] **Step 3: Run** `python -m unittest bot.tests.test_momenti -v` — Expected: i test nuovi FAIL.

- [ ] **Step 4: Implement `_da_missioni(g: str, p: dict) -> list[dict]`**
  - Un candidato per ogni evento `quest` con `action == "done"`; `dove`/`zona` come gli altri (`_dove`, `_zona`); `perche` = `Missione «{titolo}» completata` (+ ` a {dove}`).
  - Finestra delle uccisioni: episodi del giocatore con `to` in `(t_consegna − 3600, t_consegna]` e dopo la consegna precedente dello stesso giocatore (le uccisioni contano per una sola missione).
  - Con `obj`: tieni i `kills` il cui nome (minuscolo) compare nel testo degli obiettivi (minuscolo), anche con `s` finale; senza `obj` o nessuna corrispondenza: tieni i `kills` degli episodi della finestra con la stessa sottozona dell'evento della consegna (`_evento_a(p, to, "combat")`), limitati agli ultimi 30 minuti, e metti `nemici_presunti: True`. Somma per nome, ordina per `n` decrescente, max 3.
  - Punti: 20 se c'è `desc`, altrimenti 12.
  - In `candidati`, dopo `_unisci` e prima dell'ordinamento: per ogni giocatore tieni al massimo 2 candidati `missione` in cui compare in `chi` (ordine: punti, poi `xp`, poi `quando`); un candidato in gruppo resta se è tra i primi 2 di almeno un giocatore.
  - `riassunto_md`: per i candidati con `trama`/`obiettivi`/`consegna` aggiungi le tre righe dedicate ed escludi quelle chiavi da `_riga_fatti`.

- [ ] **Step 5: Run** `python -m unittest discover -s bot/tests -t .` — Expected: PASS.

- [ ] **Step 6: Commit** `git add bot/cronista/momenti.py bot/tests && git commit -m "feat(bot): missioni completate come momenti, con nemici e trama"`

---

### Task 4: Livelli bassi e bottino verde

**Files:**
- Modify: `bot/cronista/momenti.py` (`_da_eventi_semplici` per i livelli; fonte del drop ~righe 262–316; `QUALITA[2] = "non comune"`; `NOMI_TIPO["livelli"] = "Livelli"`)
- Test: `bot/tests/test_momenti.py` (aggiorna `test_duello_livello_spesa_missione`)

**Interfaces:**
- Produces: candidato `tipo="livelli"` (uno per giocatore per giornata) con `fatti {"da": int, "a": int}`, punti 15, `quando` = ora dell'ultimo livello < 10; `drop` verde con `fatti["qualita"] = "non comune"`, punti 8.

- [ ] **Step 1: Write the failing tests**
  - `test_livelli_bassi_un_solo_momento`: eventi `level` 2,3,4,5,6 → un candidato `livelli` con `fatti == {"da": 1, "a": 6}`, punti 15; nessun candidato `livello`.
  - `test_livello_10_resta_separato`: `level` 9, 10 → `livelli` `{da: 8, a: 9}` + `livello` 10 (punti 20).
  - `test_verde_solo_ai_livelli_bassi`: loot `q=2` a livello 7 → un `drop` da 8 punti; secondo verde nella giornata → ignorato; verde a livello 16 → ignorato; il livello si ricava dall'ultimo evento `level` prima del drop o da `sessione.level`.
  - aggiorna `test_duello_livello_spesa_missione`: con livelli 40/41/60 l'attesa resta `[20, 70]` (nessun `livelli`: sono ≥ 10).

- [ ] **Step 2: Run** — Expected: FAIL.

- [ ] **Step 3: Implement** — `da` = livello prima del primo `level` < 10 della giornata (evento − 1); il livello corrente del giocatore serve anche al drop: helper `_livello_a(p: dict, t: int) -> int | None` (ultimo `level` con `t` ≤ dato, altrimenti `level` della sessione; per questo `_cronologia` salva anche `p["livello_iniziale"]`, il `level` della prima sessione).

- [ ] **Step 4: Run** `python -m unittest discover -s bot/tests -t .` — Expected: PASS.

- [ ] **Step 5: Commit** `git commit -am "feat(bot): livelli sotto il 10 e bottino verde ai livelli bassi"`

---

### Task 5: Istruzioni del Cronista e prova sui dati veri

**Files:**
- Modify: `bot/ISTRUZIONI.md`, `docs/guida-bot.md` (10:07 → 07:15)
- Test: `bot/tests/test_cli.py` (prova completa)

- [ ] **Step 1: Write the failing test** `test_prepara_deathknell`: con il finto Discord caricato con la fixture del Task 3 (come pacchetto), `prepara` produce un riassunto con almeno un candidato `Missione`, un `Livelli` (1 → 6 se nei dati), le due morti, e `bilancio-2026-11-04.md` con la riga di Sciatalgia (`livello 1 → 6`, `6 missioni`, nemici > 0, `2 morti`).

- [ ] **Step 2: Run** — Expected: PASS se i Task 1–4 sono giusti; se FAIL, correggere lì (non nel test).

- [ ] **Step 3: Aggiorna `ISTRUZIONI.md`** (sezione regole del racconto), testo da aggiungere:

```
- **Missioni:** sono momenti come gli altri. Se nei Fatti c'è la **Trama**, riassumila in italiano in una o due
  frasi (nomi di persone, luoghi e nemici restano come sono). Se la trama non c'è, racconta solo il titolo, il
  luogo e i nemici abbattuti: **non aggiungere la storia della missione** da quello che sai del gioco. Se c'è
  `nemici presunti: sì`, i nemici sono **probabilmente** quelli della missione.
- Al massimo **una missione per giocatore** nel racconto, se ci sono altri momenti.
- **Bilancio:** in fondo alla pagina lo aggiunge `pubblica` da solo. Non scriverlo e non ripeterne i numeri
  come elenco.
```

- [ ] **Step 4: Run** tutti i test del bot — Expected: PASS.

- [ ] **Step 5: Commit** `git commit -am "docs(bot): istruzioni per missioni e bilancio; orario 07:15"`

**Checkpoint (dopo Task 5):** merge del ramo su `main` e push, così il Cronista di domattina usa missioni, livelli e bilancio. Prima del merge: `python bot/cronista.py pubblica --prova` non serve (richiede Discord); basta la suite verde.

---

### Task 6: Addon 0.2.0 — testi della missione e verdi sotto il 20

**Files:**
- Modify: `addon/DiarioGilda/Main.lua` (handler `QUEST_COMPLETE`, `QUEST_TURNED_IN`; registrazione eventi), `addon/DiarioGilda/Core/Recorder.lua` (`onQuestDone(id, title, xp, money, testi)`, soglia qualità), `addon/DiarioGilda/Core/Const.lua` (`VERSION = "0.2.0"`, `MAX_DESC = 600`, `MAX_OBJ = 300`, `MAX_FINE = 400`, `GREEN_MAX_LEVEL = 20`), `addon/DiarioGilda/DiarioGilda.toc` (`## Version: 0.2.0`), `docs/verifica-al-lancio.md` (voce 16 + versione 0.2.0)
- Test: `tests/lua/spec/main_spec.lua`, `tests/lua/spec/recorder_world_spec.lua`, `tests/lua/spec/recorder_items_spec.lua`

**Interfaces:**
- Produces: evento `quest` `done` con campi opzionali `desc`, `obj`, `fine` (stringhe troncate). Recorder: `onQuestDone(id, title, xp, money, testi)` dove `testi = {desc=?, obj=?, fine=?}` o nil.

- [ ] **Step 1: Write the failing tests**
  - `main_spec`: con stub `GetQuestID() → 42`, `GetRewardText() → "Well done."`, e lettura dal registro (stub `C_QuestLog.GetLogIndexForQuestID(42) → 3`, `SelectQuestLogEntry`, `GetQuestLogQuestText() → "Long story", "Kill 8 rats"`) → `H.fire("QUEST_COMPLETE")` poi `H.fire("QUEST_TURNED_IN", 42, 450, 1200)` → evento con `desc="Long story"`, `obj="Kill 8 rats"`, `fine="Well done."`.
  - API assenti (stub nil) → evento come oggi, nessun errore in `errors`.
  - `QUEST_TURNED_IN` di un altro id (o senza `QUEST_COMPLETE` prima) → nessun testo attaccato.
  - Testi più lunghi dei limiti → troncati a 600/300/400.
  - `recorder_items_spec`: oggetto verde (`q=2`) con `UnitLevel` 12 → registrato; con livello 25 → no; blu sempre.

- [ ] **Step 2: Run** `bash scripts/test-lua.sh` — Expected: FAIL.

- [ ] **Step 3: Implement** — `QUEST_COMPLETE`: `pendingQuest = { id = GetQuestID(), fine = GetRewardText(), desc, obj = <dal registro> }` tutto con `Safe.call` e `api(...)` come `questTitle`; ripristinare la selezione precedente del registro (`GetQuestLogSelection` / `SelectQuestLogEntry`) se cambiata. `QUEST_TURNED_IN(id, …)`: attacca i testi solo se `pendingQuest.id == id`, poi `pendingQuest = nil`. Troncamento con `string.sub` in `Safe` (helper `DG.Safe.cut(s, n)`). Recorder: soglia `q >= 3 or (q == 2 and level < GREEN_MAX_LEVEL)` con `level` da `self.deps.level()` (nuova dipendenza = `UnitLevel("player")`).

- [ ] **Step 4: Run** `bash scripts/test-lua.sh && go test ./...` — Expected: PASS (i test Go che confrontano la versione addon/TOC devono passare con 0.2.0).

- [ ] **Step 5: Aggiungi a `docs/verifica-al-lancio.md`** la voce 16: «Testi delle missioni: consegna una missione, `/reload`, nel file l'evento `quest done` ha `desc`, `obj`, `fine`. Se mancano: annota quali API esistono (`/dump GetQuestLogQuestText, C_QuestLog.GetLogIndexForQuestID, GetRewardText`).»

- [ ] **Step 6: Commit** `git commit -am "feat(addon): testi della missione alla consegna e verdi sotto il 20 (0.2.0)"`

**Checkpoint (dopo Task 6):** chiedere all'utente l'ok per merge + tag `v0.2.0` (release firmata, aggiornamento automatico per tutti).
