# Video del Cronista Goblin anche su WhatsApp — design (breve)

Data: 2026-11-20 · Approccio scelto dall'utente: **A**

## Perché
Il video narrato della pagina del Cronista esce ogni mattina solo in #diario-di-gilda (Discord).
L'utente lo vuole anche nel gruppo WhatsApp **Orda Florentiae**, passando dal bot riassunti che già
porta nel gruppo gli avvisi del Cancelliere. **Solo il video** con una riga di didascalia, niente testo della pagina.

## Obiettivi / non obiettivi
- Sì: ogni video pubblicato in #diario-di-gilda (non le prove in #diario-servizio) arriva una volta nel gruppo.
- Sì: rispetta il silenzio 00:30–08:00 del ponte → di solito arriva alle 08:00.
- No: testo della pagina, video vecchi, invio in altri gruppi.
- Anti-obiettivo: un errore di WhatsApp/Cancelliere non deve mai far fallire o ritardare la pubblicazione su Discord.

## Approcci considerati
- **A (scelto)** — `saggio.yml` dopo aver pubblicato avvisa il Cancelliere con l'URL del video; il Cancelliere lo
  mette nella coda `wa_outbox`; il bot WhatsApp scarica e invia. Evento puntuale, una sola coda con le regole esistenti.
- B — il Cancelliere controlla #diario-di-gilda a intervalli: nessuna modifica al Saggio, ma interroga Discord per un
  evento al giorno e serve il permesso di lettura sul canale.
- C — il bot WhatsApp legge Discord da solo: token Discord su Oracle e regole d'orario duplicate. Scartato.

## Design
**1. Saggio (`bot/saggio/narra.py`, `saggio.yml`, repo diario-gilda)**
- Dopo `d.invia(...)` riuscito e **solo se non è `--prova`**: POST a `https://orda-bottega.oldpz1.workers.dev/wa/cronaca`
  con `{ref: <id pagina>, titolo: <titolo pagina>, video_url: <attachments[0].url della risposta Discord>}`,
  header `Authorization: Bearer $WA_CRONACA_SECRET`.
- Errore (rete, 4xx/5xx, secret mancante) = una riga di avviso nel log, il job resta verde.
- Nuovo secret GitHub `WA_CRONACA_SECRET`. `d.invia` deve restituire il messaggio creato (se oggi non lo fa).

**2. Il Cancelliere (`bottega`, Cloudflare Worker)**
- Migrazione 0011: colonna `media_url TEXT` (nullable) in `wa_outbox`.
- Nuovo tipo `"cronaca"` in `WaKind`, **non conteggiato** nel tetto di 6 al giorno (come `announce`), da aggiungere a `WA_NOTIFY`.
- `POST /wa/cronaca` con un **segreto suo** (`WA_CRONACA_SECRET`, diverso da `WA_BRIDGE_SECRET`, così il workflow non può
  leggere né confermare la coda). Valida: `ref` numerico, `titolo` ≤ 200 car., `video_url` https su
  `cdn.discordapp.com` o `media.discordapp.net`. `enqueue` con `ref` = id pagina (doppioni ignorati),
  `expires_at` = adesso + 20 h (l'URL di Discord scade in ~24 h).
- Testo/didascalia: `HEADER` + `📜 <titolo>` + riga breve tipo «la voce del Cronista Goblin».
- `/wa/pull` aggiunge `video` (l'URL) agli avvisi che lo hanno; gli altri restano identici.

**3. Bot riassunti (`bot recap whatsapp`, Oracle)**
- `BridgeNotice.video?: string`. Se presente: scarica l'URL (timeout 60 s, massimo 25 MB), invia
  `{ video: buffer, mimetype: "video/mp4", caption: text }`; altrimenti testo come oggi.
- Download o invio fallito → stessa regola di oggi: niente ack, si riprova al giro dopo (fino alla scadenza).
- L'eco della didascalia inizia con `HEADER`, quindi `isBridgeEcho` la riconosce già (da verificare che legga anche le didascalie).

**Ordine di rilascio:** bot su Oracle → migrazione + deploy Worker + secret Cloudflare → secret GitHub + Saggio.
Così nessuno dei tre riceve un campo che non sa trattare.

**Prova:** inserire a mano un avviso `cronaca` col video di oggi (fuori dal silenzio) e vederlo arrivare nel gruppo;
poi il giro vero del mattino dopo.

## Lasciato al costruttore
Nomi esatti di funzioni/variabili, testi dei log, test unitari nei tre progetti secondo le loro abitudini.
