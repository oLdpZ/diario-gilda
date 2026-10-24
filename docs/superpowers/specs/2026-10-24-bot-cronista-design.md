# Diario di Gilda — Il Cronista (bot del diario) — Design

Data: 2026-10-24 · Autore: oLd_pZ con Claude · Estende: `2026-10-23-diario-di-gilda-design.md` (sezione "Bot del diario")

L'autore ha chiesto di non fare domande e di scegliere il meglio: tutte le scelte qui sotto sono di Claude,
dentro i confini già approvati nel design principale (una pagina al giorno, 3–5 momenti, narratore epico
alla WoW, ore 10:00, giornata 06:00–06:00, attività programmata di Claude Code nel cloud, stato su Discord).

## Come gira

- **Attività programmata di Claude Code** (claude.ai/code/routines) sul repository `oLdpZ/diario-gilda`,
  ogni giorno alle **10:07** ora italiana (i minuti tondi possono partire in ritardo). Usa l'abbonamento
  dell'autore. Il prompt dell'attività è una riga: «Segui `bot/ISTRUZIONI.md`».
- **Ambiente cloud**: accesso di rete *Custom* con `discord.com`, `cdn.discordapp.com` e
  `media.discordapp.net` (gli allegati si scaricano da questi due), più i gestori di pacchetti predefiniti.
  Il token del bot è un **Network secret** solo per `discord.com`, header `Authorization`, prefisso `Bot`:
  il proxy lo aggiunge alle richieste, il token non è mai nell'ambiente né nei file, né va verso i domini
  degli allegati.
- **Nulla viene scritto nel repository** (è pubblico): niente dati dei giocatori, niente pagine, niente
  stato. Tutto vive su Discord.

## Pezzi

1. **`bot/cronista.py`** — strumento Python 3 (solo libreria standard), sottocomandi:
   - `prepara`: legge lo stato, scarica da `#diario-servizio` i file nuovi (pacchetti `sessione_*.json.gz`
     del programmino e, come riserva, `DiarioGilda.lua` caricati a mano), li decodifica, scarta i doppioni
     (stessa `sessione.id`: tiene la copia con più eventi, poi lo `stop` più recente), assegna ogni
     sessione alla giornata di gioco in cui è iniziata (06:00–06:00 Europe/Rome), e per ogni giornata non
     ancora pubblicata con dati scrive `lavoro/giorno-AAAA-MM-GG.json` e un riassunto leggibile
     `lavoro/giorno-AAAA-MM-GG.md` con i **momenti candidati** già calcolati e ordinati per interesse.
   - `pubblica <pagina.md> [--prova]`: pubblica la pagina in `#diario-di-gilda` (o in `#diario-servizio` con
     `--prova`) come uno o più embed nei limiti di Discord.
   - `chiudi`: aggiorna lo stato (giorni pubblicati, numero del giorno, ultimo messaggio letto, sessioni già
     viste, contatori, ricorrenze) e lo carica come allegato `stato.json` in `#diario-servizio`.
   - Il token: se c'è `DISCORD_BOT_TOKEN` (prove in locale) lo usa, altrimenti si affida al proxy.
   - I canali si trovano per nome (`diario-di-gilda`, `diario-servizio`) nel server dove il bot è invitato
     (nome reale: diario-di-gilda (R3));
     `bot/config.json` contiene solo nomi, fuso orario e orari, niente segreti.
2. **Momenti candidati** (in `prepara`): punteggi semplici e spiegati, per esempio drop epico/leggendario,
   boss ucciso (con quanti tentativi), wipe ripetuti, morti (soprattutto caduta/annegamento o tante nella
   stessa zona), fughe (combattimento con ≥4 nemici e vita ≤20%), pesca seguita da combattimento,
   salvataggi all'ultimo (vita ≤10% e cure ricevute), duelli tra membri, livelli tondi e il 60, prime visite,
   spese folli. **Episodi condivisi**: stesso incontro (id) o stessa zona entro pochi minuti per più membri
   diventano un solo candidato con tutti i nomi. I contatori nello stato permettono "al 40° tentativo".
3. **`bot/ISTRUZIONI.md`** — il copione per Claude: esegui `prepara`; per ogni giornata pronta leggi il
   riassunto, scegli 3–5 momenti (varietà di giocatori, almeno uno buffo se c'è), scrivi la pagina con la
   voce del narratore (guida di stile con l'esempio approvato), salva `lavoro/pagina-AAAA-MM-GG.md`,
   `pubblica`, poi `chiudi`. Regole: solo fatti presenti nei dati, nomi dei giocatori come nel diario, nomi
   di gioco come li dà il client, titolo «Cronache della Gilda — Giorno N», niente menzioni @ (nessun ping),
   niente pagina se non ci sono dati. Se mancano più giorni la prima volta: una sola pagina di recupero
   «Le Cronache dei primi giorni». Ricorrenze: Claude può aggiungere allo stato brevi note ("Gino e i Murloc")
   da richiamare nei giorni dopo.
4. **`docs/guida-bot.md`** — per l'autore: creare l'applicazione bot, invitarla nel server con i permessi
   minimi, mettere il token nel Network secret, creare l'attività programmata, fare la prova.

## Prova

Test automatici (`python -m unittest` in `bot/tests`) con un finto server Discord locale e pacchetti di
esempio (la fuga dai Murloc e una serata di dungeon con due giocatori). Prova reale: in locale con il token
negli appunti, `pubblica --prova` in `#diario-servizio`; poi "Esegui ora" dell'attività programmata.

## Scelte di Claude

Python standard library (presente nel cloud e sul PC, nessuna installazione); stato come allegato su
Discord; 10:07 invece di 10:00; nessuna menzione @; il punteggio dei momenti è un aiuto, la scelta
finale la fa Claude scrivendo.
