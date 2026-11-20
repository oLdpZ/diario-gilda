# Video del Cronista su WhatsApp — piano

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ogni video della cronaca pubblicato in #diario-di-gilda arriva una volta nel gruppo WhatsApp Orda Florentiae.

**Architecture:** il Saggio (GitHub Actions) dopo la pubblicazione fa POST `/wa/cronaca` al Worker del Cancelliere con l'URL dell'allegato Discord; il Worker lo mette in `wa_outbox` (kind `cronaca`, colonna nuova `media_url`); `/wa/pull` lo espone come `video`; il bot WhatsApp scarica l'URL e lo manda come video con didascalia.

**Tech Stack:** Python stdlib (Saggio), Cloudflare Worker TS + D1 + vitest-pool-workers (bottega), Node/TS + Baileys + node:test (bot WhatsApp).

**Spec:** `docs/superpowers/specs/2026-11-20-video-whatsapp-design.md`

**Repository:**
- R1 = `C:\Users\old_p\Documents\mod guild journal` (diario-gilda, GitHub oLdpZ/diario-gilda)
- R2 = `C:\Users\old_p\Documents\server discord wow forever\bottega` (Worker, push su GitHub)
- R3 = `C:\Users\old_p\Documents\bot recap whatsapp` (solo locale, deploy su Oracle `ubuntu@89.168.26.137`, chiave `~/.ssh/oracle_bot`, servizio `whatsapp-recap`)

## Global Constraints
- Solo i video veri (non `--prova`) vanno a WhatsApp.
- Un errore verso il Cancelliere non fa mai fallire il job del Saggio (solo una riga di log).
- Kind `cronaca` **non conteggiato** nel tetto giornaliero (counted = 0), soggetto al silenzio 00:30–08:00.
- Segreto `WA_CRONACA_SECRET` distinto da `WA_BRIDGE_SECRET`.
- `video_url` accettato solo `https://cdn.discordapp.com/…` o `https://media.discordapp.net/…`.
- `expires_at` = adesso + 20 h (72000 s). `ref` = id della pagina Discord (doppioni ignorati).
- Bot: download con timeout 60 s e massimo 25 MB (26214400 byte); mimetype `video/mp4`.
- Didascalia = `HEADER` del Worker (`"🤖 *Il Cancelliere*\n"`) + `📜 <titolo>` + `\nLa voce del Cronista Goblin 🎙️`.
- Mai stampare segreti; copia del segreto nuovo in `C:\Users\old_p\Documents\Diario di Gilda - chiavi\chiavi.txt`.

## Review Focus
1. Il bot si riavvia e rivede la propria didascalia «[video] 🤖 *Il Cancelliere*…»: non va riassunta né trattata come comando → `isBridgeEcho` deve riconoscerla (test in Task 1).
2. URL scaduto o 404 al momento del download: niente ack, log di avviso, si riprova fino alla scadenza (test in Task 1).
3. Video più grande di 25 MB: non si invia, niente crash, avviso (test in Task 1).
4. Lo stesso video segnalato due volte (giro manuale + cron): un solo avviso in coda (test in Task 2).
5. Workflow senza secret `WA_CRONACA_SECRET`: salta in silenzio con un avviso, il job resta verde (test in Task 3).

---

### Task 1: bot WhatsApp manda i video del ponte (R3)

**Files:**
- Modify: `src/cancelliere.ts` (`BridgeNotice.video?: string`; `pull` mantiene `video` solo se stringa)
- Modify: `src/bridge.ts` (`send` riceve il notice intero; `isBridgeEcho` accetta anche il prefisso `"[video] "`)
- Create: `src/media.ts` — `downloadVideo(url: string, opts?: { fetchImpl?: typeof fetch; maxBytes?: number; timeoutMs?: number }): Promise<Buffer>` (lancia errore su status non ok, Content-Length o corpo oltre il massimo)
- Modify: `src/index.ts:454-462` (se `n.video`: `sendMessage(group, { video: await downloadVideo(n.video), mimetype: "video/mp4", caption: n.text, mentions })`, altrimenti testo come oggi)
- Test: `test/bridge.test.ts`, `test/media.test.ts`

**Interfaces:**
- Produces: `BridgeDeps.send: (n: BridgeNotice) => Promise<string | undefined>`; `downloadVideo` come sopra.
- Consumes: campo `video` di `/wa/pull` (Task 2).

- [ ] **Step 1: test che falliscono**
  - `bridge.test.ts`: adattare `setup` al nuovo `send(n)`; nuovo test «notice with video is passed whole to send» (`send` riceve `{id:1,text:"c",mentions:[],video:"https://cdn.discordapp.com/a.mp4"}`); test «send failure on a video notice: no ack, retried next tick» (prima tick fallisce → `acked` vuoto, seconda riesce → `[[1]]`).
  - `isBridgeEcho("[video] 🤖 *Il Cancelliere*\n📜 X") === true`, `isBridgeEcho("[video] ciao") === false`.
  - `Cancelliere.pull` con risposta `{messages:[{id:1,text:"t",mentions:[],video:"https://…"},{id:2,text:"u",mentions:[],video:5}]}` → primo ha `video`, secondo no.
  - `media.test.ts`: fetch finto 200 con 3 byte → Buffer di 3; 404 → rejects; Content-Length 30 MB → rejects senza leggere; corpo oltre `maxBytes: 10` senza Content-Length → rejects.
- [ ] **Step 2:** `npm test` → FAIL sui test nuovi.
- [ ] **Step 3:** implementare come in Files.
- [ ] **Step 4:** `npm test` e `npm run typecheck` → tutto verde.
- [ ] **Step 5: commit** `feat(ponte): avvisi con video (cronaca del Cronista Goblin)`.

### Task 2: il Cancelliere accoda la cronaca (R2)

**Files:**
- Create: `migrations/0011_wa_media.sql` — `ALTER TABLE wa_outbox ADD COLUMN media_url TEXT;`
- Modify: `src/whatsapp/queue.ts` — `WaKind` + `"cronaca"`; `enqueue` accetta `mediaUrl?: string` e scrive `media_url`; counted = 0 anche per `cronaca`; `pullBatch` restituisce `video` quando `media_url` non è null; `PRIORITY` invariata (gli uncounted escono per primi).
- Create: `src/whatsapp/cronaca.ts` — `cronacaNotice(titolo: string): string` (didascalia dei Global Constraints) e `validCronaca(body: unknown): { ref: string; titolo: string; videoUrl: string } | null` (ref solo cifre ≤ 25, titolo stringa 1–200 car., URL dei due host).
- Modify: `src/whatsapp/routes.ts` — `POST /wa/cronaca` controllato **prima** del controllo `WA_BRIDGE_SECRET`, con `safeEqual` su `Bearer ${env.WA_CRONACA_SECRET}` (401 se vuoto/errato), 400 se non valido, `enqueue({kind:"cronaca", ref, text: cronacaNotice(titolo), mediaUrl: videoUrl, expiresAt: now+72000})` → `{ok:true, queued:boolean}`.
- Modify: `src/index.ts` (`WA_CRONACA_SECRET: string` in `Env`), `wrangler.toml` e `vitest.config.ts` (`WA_NOTIFY` + `,cronaca`; in vitest `WA_CRONACA_SECRET: "test-cronaca-secret"`).
- Test: `test/wa-routes.test.ts`, `test/wa-queue.test.ts`

**Interfaces:**
- Produces: `POST /wa/cronaca` body `{ref, titolo, video_url}`; `/wa/pull` → `messages[i].video?: string`.

- [ ] **Step 1: test che falliscono** (`wa-routes.test.ts`):
  - 401 con il bearer del ponte e senza bearer; 401 se `WA_CRONACA_SECRET` vuoto.
  - 400 con `video_url` `https://evil.example/a.mp4`, con `ref` non numerico, con titolo vuoto.
  - POST valido → `queued: true`; ripetuto con stesso `ref` → `queued: false` e una sola riga in `wa_outbox`.
  - poi `GET /wa/pull` (bearer ponte, fuori silenzio: usare `vi.setSystemTime` su un'ora 10:00 Roma come fanno i test esistenti) → messaggio con `video` uguale all'URL e `text` che inizia con `"🤖 *Il Cancelliere*\n📜 "`.
  - `wa-queue.test.ts`: con 6 avvisi conteggiati già consegnati oggi, una `cronaca` in coda esce lo stesso; durante il silenzio non esce.
- [ ] **Step 2:** `npm test -- wa-` → FAIL.
- [ ] **Step 3:** implementare.
- [ ] **Step 4:** `npm test` e `npm run typecheck` → verde.
- [ ] **Step 5: commit** `feat(whatsapp): video della cronaca dal Saggio (/wa/cronaca)`.

### Task 3: il Saggio avvisa il Cancelliere (R1)

**Files:**
- Create: `bot/saggio/whatsapp.py` — `annuncia(ref: str, titolo: str, messaggio: dict) -> bool`: prende `messaggio["attachments"][0]["url"]`; se manca `WA_CRONACA_SECRET` o l'allegato stampa un avviso e torna False; POST JSON a `(os.environ.get("CANCELLIERE_URL") or "https://orda-bottega.oldpz1.workers.dev") + "/wa/cronaca"` con `Authorization: Bearer …`, timeout 30 s, via `_APRI` e `USER_AGENT` di `bot/cronista/discord.py` (come `avvio_saggio._chiama`); stampa «  WhatsApp: in coda» / «  WhatsApp: già in coda» / «  WhatsApp NON avvisato: <codice o errore>»; non lancia mai.
- Modify: `bot/saggio/narra.py:105-108` — salvare il dict restituito da `d.invia(...)` e, se non `a.prova`, chiamare `annuncia(p["id"], p["titolo"], msg)` (fuori dal `try` della pubblicazione, così un'eccezione imprevista non conta come video non pubblicato: dentro `annuncia` ogni errore è già gestito).
- Modify: `.github/workflows/saggio.yml` — env `WA_CRONACA_SECRET: ${{ secrets.WA_CRONACA_SECRET }}`.
- Test: `bot/tests/test_saggio_whatsapp.py`

- [ ] **Step 1: test che falliscono** (unittest, `_APRI` sostituito con `unittest.mock.patch("bot.saggio.whatsapp._APRI")`):
  - senza secret → False, nessuna chiamata.
  - messaggio senza `attachments` → False, nessuna chiamata.
  - risposta 200 `{"ok":true,"queued":true}` → True; la richiesta ha URL `…/wa/cronaca`, header `Authorization: Bearer s3cr3t`, corpo `{"ref":"123","titolo":"Cronache — Giorno 16","video_url":"https://cdn.discordapp.com/x.mp4"}`.
  - `HTTPError` 401 e `OSError` → False, nessuna eccezione.
- [ ] **Step 2:** `python -m unittest bot.tests.test_saggio_whatsapp -v` → FAIL (modulo mancante).
- [ ] **Step 3:** implementare.
- [ ] **Step 4:** `python -m unittest discover -s bot/tests -t .` → tutto verde.
- [ ] **Step 5: commit** su branch `feat/video-whatsapp` `feat(saggio): il video della cronaca va anche su WhatsApp (via il Cancelliere)`, poi merge su main.

### Task 4: rilascio e prova (operativo, nell'ordine)

- [ ] **Step 1:** generare il segreto (`python -c "import secrets;print(secrets.token_urlsafe(32))"` in un file nello scratchpad, mai a schermo), copiarlo in `chiavi.txt` come `WA_CRONACA_SECRET=…`.
- [ ] **Step 2 (R3):** copia di sicurezza `~/bot/src.bak-video` su Oracle, `scp` di `src/`, `sudo systemctl restart whatsapp-recap`, `journalctl -u whatsapp-recap -n 30` → «ponte con Il Cancelliere attivo», nessun errore.
- [ ] **Step 3 (R2):** `npx wrangler d1 migrations apply <db> --remote` (0011), `npx wrangler secret put WA_CRONACA_SECRET` (da file), `npm run deploy`, push su GitHub.
- [ ] **Step 4 (R1):** `gh secret set WA_CRONACA_SECRET < file`, push di main.
- [ ] **Step 5: prova vera:** con `curl` POST `/wa/cronaca` con `ref` = id della pagina di oggi e URL dell'allegato del video di oggi (letto da Discord col token del bot) → `queued:true`; entro ~60 s il video compare nel gruppo con la didascalia. Cancellare il file del segreto dallo scratchpad.
- [ ] **Step 6:** domattina (21/11) controllare: log del run Saggio «WhatsApp: in coda», video nel gruppo alle ~08:00.
- [ ] **Step 7:** aggiornare `STATO-SERVER.md` (sezione Ponte WhatsApp) e la nota di memoria.
