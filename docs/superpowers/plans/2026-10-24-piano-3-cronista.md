# Diario di Gilda — Piano 3: il Cronista Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Uno strumento Python che, guidato da Claude in un'attività programmata nel cloud, trasforma i pacchetti delle sessioni in `#diario-servizio` in una pagina di diario al giorno in `#diario-gilda`.

**Architecture:** `bot/cronista.py` (CLI, solo stdlib) con moduli piccoli in `bot/cronista/`: `discord.py` (REST), `stato.py`, `ingresso.py` (scarico + decodifica + doppioni + giornate), `momenti.py` (candidati e punteggi), `pagina.py` (embed). Claude fa la parte creativa seguendo `bot/ISTRUZIONI.md`.

**Tech Stack:** Python 3.11+ standard library (`urllib.request`, `json`, `gzip`, `zoneinfo`, `argparse`, `unittest`, `http.server` per i test).

**Spec:** `docs/superpowers/specs/2026-10-24-bot-cronista-design.md` · formato dei pacchetti: `docs/formato-dati.md` (sezione "File inviato a Discord") · design generale `docs/superpowers/specs/2026-10-23-diario-di-gilda-design.md`.

## Global Constraints

- Solo libreria standard di Python ≥ 3.11; nessun `pip install`. Test: `python -m unittest discover -s bot/tests -t .` dalla radice.
- Mai scrivere dati dei giocatori, pagine o stato nel repository: la cartella di lavoro è `bot/lavoro/` ed è in `.gitignore`.
- Discord REST v10, base `https://discord.com/api/v10` (sovrascrivibile con `CRONISTA_API` per i test). Header `Authorization: Bot <DISCORD_BOT_TOKEN>` **solo se** la variabile esiste; altrimenti nessun header (lo aggiunge il proxy del cloud). Il token non compare mai in output, errori o file. `User-Agent: DiarioGilda-Cronista (https://github.com/oLdpZ/diario-gilda, 0.1)`.
- Limiti Discord: rispettare HTTP 429 (`retry_after` nel JSON, max 3 tentativi, attesa max 30 s); embed: titolo ≤ 256, descrizione ≤ 4096, max 10 embed e 6000 caratteri totali per messaggio; allegati ≤ 8 MB.
- `bot/config.json`: `{"server": null, "canale_diario": "diario-gilda", "canale_servizio": "diario-servizio", "fuso": "Europe/Rome", "inizio_giornata": "06:00"}` (`server` = id o nome; null = l'unico server del bot).
- Giornata di gioco: dalle 06:00 alle 06:00 del giorno dopo, Europe/Rome; una sessione appartiene alla giornata del suo `start`.
- Doppioni: stessa `sessione.id` → tieni la copia con più eventi, a parità lo `stop` più recente.
- Testi per i giocatori in italiano; nessuna menzione `@`; `allowed_mentions: {"parse": []}` su ogni messaggio.
- Lo stato vive in `#diario-servizio` come allegato `stato.json` in un messaggio che inizia con `Stato del Cronista`; si legge l'ultimo così fatto.

## Review Focus

- Primo avvio con pacchetti di più giorni e nessuno stato: una sola pagina di recupero, poi lo stato dice che quei giorni sono fatti (test in Task 2 e 4).
- Un pacchetto corrotto o un `.lua` illeggibile non blocca gli altri: viene saltato con un avviso nel riassunto (test in Task 2).
- Paginazione: più di 100 messaggi nuovi nel canale di servizio vengono letti tutti (test in Task 1).
- Pagina troppo lunga per un embed: divisa senza spezzare le parole e senza superare i 6000 caratteri per messaggio (test in Task 4).
- Il token non compare in nessun errore (test in Task 1).

---

### Task 1: `discord.py` e `stato.py` — client REST e stato

**Files:** Create `bot/cronista/__init__.py`, `bot/cronista/discord.py`, `bot/cronista/stato.py`, `bot/config.json`, `bot/tests/__init__.py`, `bot/tests/finto_discord.py`, `bot/tests/test_discord.py`, `bot/tests/test_stato.py`; Modify `.gitignore` (`bot/lavoro/`, `__pycache__/`), `.github/workflows/ci.yml` (job `python`: `python -m unittest discover -s bot/tests -t .`).

**Interfaces:**
- Produces: `class Discord(base: str | None = None, token: str | None = None)` (default da `CRONISTA_API` / `DISCORD_BOT_TOKEN`); metodi: `server(id_o_nome: str | None) -> dict`; `canale(server_id: str, nome: str) -> str` (id); `messaggi_dopo(canale_id: str, dopo: str | None) -> list[dict]` (tutti i messaggi più recenti di `dopo`, dal più vecchio, paginando a 100); `ultimi(canale_id: str, n: int = 100) -> list[dict]`; `scarica(url: str) -> bytes` (allegati, cap 16 MB); `invia(canale_id: str, contenuto: str = "", embed: list[dict] | None = None, file: tuple[str, bytes] | None = None) -> dict`. Errori: `DiscordError(status: int, testo: str)` senza mai il token.
- Produces: `bot/tests/finto_discord.py`: un server `http.server` in un thread che simula `GET /users/@me/guilds`, `GET /guilds/{id}/channels`, `GET /channels/{id}/messages` (con `after`/`before`/`limit`), `POST /channels/{id}/messages` (JSON e multipart con `files[0]`), allegati serviti da `/allegati/<nome>`, e può rispondere 429 una volta; registra le richieste (header compresi) per le asserzioni.
- Produces: `stato.py`: `Stato` dataclass `{versione: 1, ultimo_messaggio: str | None, giorni_pubblicati: list[str], numero_giorno: int, sessioni_viste: dict[str, int]  # id -> eventi, contatori: dict, ricorrenze: list[str]}`; `leggi(d: Discord, canale_id) -> Stato` (dall'ultimo messaggio `Stato del Cronista…` con allegato `stato.json`; nessuno → `Stato()` vuoto); `scrivi(d, canale_id, s: Stato) -> None` (messaggio `Stato del Cronista — <data>` + allegato); `sessioni_viste` potate oltre 60 giorni tramite un campo data per id (scegli la forma, documentala).

- [ ] **Step 1:** Test con il finto server: `server(None)` con un solo server; `canale` per nome; `messaggi_dopo` con 250 messaggi → 250 in ordine dal più vecchio; 429 seguito da 200 → riuscito; con `DISCORD_BOT_TOKEN="segreto"` l'header è `Bot segreto` e un 403 genera un `DiscordError` il cui `str()` non contiene `segreto`; senza variabile nessun header `Authorization`; `invia` con file → multipart con `files[0]` e `payload_json` contenente `allowed_mentions`. Stato: senza messaggi → vuoto; `scrivi` poi `leggi` → uguale.
- [ ] **Step 2:** Esegui → FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(bot): client Discord e stato del Cronista`.

### Task 2: `ingresso.py` — pacchetti, doppioni, giornate

**Files:** Create `bot/cronista/ingresso.py`, `bot/cronista/luasv.py`, `bot/tests/test_ingresso.py`, `bot/tests/dati/` (pacchetti d'esempio).

**Interfaces:**
- Consumes: Task 1 (`Discord`, `Stato`).
- Produces: `luasv.carica(testo: str) -> dict` (sottoinsieme delle SavedVariables come nel parser Go `app/internal/svparse`: tabelle, stringhe con escape, numeri, booleani, nil, commenti, profondità max 200; `{}` → lista vuota).
- Produces: `ingresso.leggi_pacchetto(nome: str, dati: bytes) -> list[dict]` → elenco di "sessioni arricchite" `{giocatore, interrotta, troncata, sessione, combattimenti}`; `.json.gz` secondo `docs/formato-dati.md`; `DiarioGilda.lua` → ogni sessione di `DiarioGildaDB.sessions` con `giocatore = nome del personaggio`, `combattimenti = []`, `manuale = True`; errori → `ValueError` con messaggio.
- Produces: `ingresso.raccogli(d: Discord, servizio_id: str, stato: Stato) -> Raccolta` con `Raccolta(sessioni: list[dict], avvisi: list[str], ultimo_messaggio: str | None)`: scarica gli allegati dei messaggi dopo `stato.ultimo_messaggio`, ignora i messaggi dello stato e quelli senza allegati, salta pacchetti corrotti con un avviso, applica i doppioni (anche contro `stato.sessioni_viste`: una sessione già vista con lo stesso numero di eventi o meno viene ignorata; una cresciuta rientra).
- Produces: `ingresso.giornata(start_epoch: int, fuso: str, inizio: str) -> str` (`AAAA-MM-GG`) e `ingresso.per_giornata(sessioni) -> dict[str, list[dict]]`.

- [ ] **Step 1:** Crea in `bot/tests/dati/` due pacchetti `.json.gz` realistici secondo `docs/formato-dati.md` (Gino: pesca → fuga dai Murloc con `minHp` 12 e un episodio con 6 nemici; Mara e Gino: Deadmines, 3 wipe e kill di Van Cleef con un drop epico), uno corrotto e un `DiarioGilda.lua` (copia di `tests/fixtures/DiarioGilda.lua`). Test: doppione con meno eventi scartato; sessione cresciuta rientra; corrotto → avviso e gli altri proseguono; sessione che inizia alle 05:59 → giornata del giorno prima, alle 06:00 → giornata stessa (Europe/Rome, anche a cavallo del cambio d'ora 25 ottobre); `.lua` letto con `manuale=True`; messaggi di stato ignorati.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(bot): raccolta dei pacchetti e giornate di gioco`.

### Task 3: `momenti.py` — candidati, episodi condivisi, contatori

**Files:** Create `bot/cronista/momenti.py`, `bot/tests/test_momenti.py`.

**Interfaces:**
- Consumes: sessioni arricchite (Task 2), `Stato.contatori`.
- Produces: `momenti.candidati(sessioni: list[dict], contatori: dict) -> list[dict]`, ordinati per `punti` decrescente; ogni candidato `{tipo, punti, quando: epoch, dove, chi: [giocatori], fatti: {...}, perche: str}`. Tipi e punti (fissati qui): `drop` (q=4: 60, q=5: 90, q=3 da un boss: 30), `boss_ucciso` (50 + 10 per ogni wipe precedente sullo stesso `id` nella giornata, `fatti.tentativi`), `wipe_ripetuti` (≥3 wipe senza kill: 40), `fuga` (episodio con `enemyCount ≥ 4` e `minHp ≤ 20` e nessuna morte entro 30 s: 55; +15 se preceduto da `fishing` entro 120 s, `fatti.pesca=true`), `salvataggio` (`minHp ≤ 10`, cure ricevute, nessuna morte: 45), `morte` (caduta/annegamento: 40; ≥3 morti nella stessa zona nella giornata: 35 come un solo candidato; altrimenti 10), `duello` (tra due giocatori del diario: 30), `livello` (60: 70; multipli di 10: 20), `prima_visita` (zona nuova: 5), `spesa` (≥100 oro: 25), `missione_abbandonata` (10). Episodi condivisi: candidati dello stesso `tipo` con stesso `encounter id`, oppure stessa zona e `quando` entro 300 s, di giocatori diversi → uniti (`chi` unione, `punti` = max + 10 per giocatore in più).
- Produces: `momenti.aggiorna_contatori(contatori: dict, sessioni) -> dict`: tentativi per (giocatore, boss) e per (giocatore, oggetto inseguito = boss il cui drop epico non è ancora arrivato) cumulativi tra i giorni; `momenti.riassunto_md(giorno: str, candidati, avvisi, giocatori) -> str`: il riassunto leggibile per Claude (intestazione con data, giocatori della giornata, poi un elenco numerato di candidati con `perche` e i fatti principali, poi gli avvisi).

- [ ] **Step 1:** Test sui pacchetti d'esempio: la fuga di Gino è un candidato `fuga` con `pesca=true` e 70 punti; Deadmines dà un `boss_ucciso` con `tentativi=4` e `chi` = Gino e Mara (unito), più il `drop` epico; ordinamento per punti; tre morti nella stessa zona → un solo candidato; contatori cumulativi su due giornate; `riassunto_md` contiene i nomi, il numero del candidato e gli avvisi.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(bot): momenti candidati della giornata`.

### Task 4: CLI, pubblicazione, istruzioni e guida

**Files:** Create `bot/cronista.py`, `bot/cronista/pagina.py`, `bot/ISTRUZIONI.md`, `docs/guida-bot.md`, `bot/tests/test_pagina.py`, `bot/tests/test_cli.py`.

**Interfaces:**
- Consumes: Task 1–3.
- Produces: `pagina.embed(testo_md: str) -> list[list[dict]]`: la prima riga `# Titolo` diventa il titolo dell'embed (≤256), il resto la descrizione; divisione a paragrafi (poi parole) in embed ≤4096, raggruppati in messaggi da ≤10 embed e ≤6000 caratteri; colore fisso `0xC8A15A`; nel primo embed `footer: {"text": "Il Cronista della Gilda"}`.
- Produces: CLI `python bot/cronista.py prepara | pubblica <file.md> [--prova] | chiudi [--ricorrenza TESTO]...`:
  - `prepara`: config + `Discord()` + stato + `raccogli`; per ogni giornata con dati e non in `giorni_pubblicati` scrive `bot/lavoro/giorno-<data>.md` e `.json`; se le giornate pronte sono più di una e `giorni_pubblicati` è vuoto le unisce in `bot/lavoro/recupero.md` (titolo «Le Cronache dei primi giorni»); salva `bot/lavoro/prepara.json` con l'elenco dei file, il numero del prossimo giorno e lo stato aggiornato in attesa; stampa un riepilogo breve (giornate pronte, giocatori, avvisi). Nessuna giornata → stampa `Niente da raccontare oggi.` ed esce 0.
  - `pubblica`: legge il file e invia gli embed al canale del diario (o di servizio con `--prova`).
  - `chiudi`: applica lo stato in attesa da `prepara.json` (giorni pubblicati, `numero_giorno`, `ultimo_messaggio`, `sessioni_viste`, contatori, ricorrenze aggiunte) e lo scrive su Discord. Senza `prepara.json` → errore chiaro.
- Produces: `bot/ISTRUZIONI.md` (italiano, rivolto a Claude nell'attività programmata): i passi esatti (`prepara` → leggi i `.md` → scrivi una pagina per giornata o la pagina di recupero → `pubblica` → `chiudi`), la guida di stile (narratore epico alla WoW, solenne anche per le figuracce, esempio approvato dal design generale), le regole (3–5 momenti, solo fatti dei dati, varietà di giocatori, almeno un momento buffo se c'è, titolo «Cronache della Gilda — Giorno N, Era della Rinascita», lunghezza 1200–2500 caratteri, nessuna @, nomi dei giocatori come nei dati, nomi di gioco come nel client), cosa fare se un comando fallisce (non pubblicare, riportare l'errore nella risposta finale), e le ricorrenze (`--ricorrenza` per richiami futuri, max 3 al giorno, brevi).
- Produces: `docs/guida-bot.md` (per l'autore): creare l'applicazione su https://discord.com/developers/applications, bot con **Message Content Intent** attivo (senza, l'API REST restituisce gli allegati dei messaggi del webhook vuoti; sotto i 100 server basta l'interruttore, nessuna verifica), invito con permessi `View Channel`, `Read Message History`, `Send Messages`, `Embed Links`, `Attach Files` (link OAuth2 con `scope=bot` e `permissions=117760`); su claude.ai/code/routines: ambiente con rete *Custom* `discord.com` + Network secret (`discord.com`, header `Authorization`, prefisso `Bot`), nuova attività sul repo `oLdpZ/diario-gilda`, ogni giorno alle 10:07, prompt «Segui bot/ISTRUZIONI.md»; prova con «Esegui ora»; prova locale con `DISCORD_BOT_TOKEN` e `pubblica --prova`.

- [ ] **Step 1:** Test `pagina`: testo di 9000 caratteri → più embed, nessuno oltre 4096, nessun messaggio oltre 6000, nessuna parola spezzata, titolo dal `#`. Test CLI end-to-end col finto server: pacchetti d'esempio caricati in `#diario-servizio` → `prepara` scrive i file attesi; `pubblica` invia embed con `allowed_mentions` vuoto; `chiudi` scrive lo stato; un secondo `prepara` → `Niente da raccontare oggi.`; primo avvio con due giornate → `recupero.md`.
- [ ] **Step 2:** FAIL. **Step 3:** Implementa. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(bot): comandi del Cronista, istruzioni e guida`.
