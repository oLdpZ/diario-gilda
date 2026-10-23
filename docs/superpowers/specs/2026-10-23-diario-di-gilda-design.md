# Diario di Gilda per WoW Forever — Design

Data: 2026-10-23 · Autore: oLd_pZ con Claude

## Intento

Una gilda di 10 amici su **World of Warcraft: Forever** (uscita 5 novembre 2026, 00:00 CET) vuole un
**diario di gilda scritto in automatico**: ogni mattina, nel canale Discord della gilda, una pagina che racconta
le imprese del giorno prima, dalle più buffe alle più epiche. L'esempio che ha dato il via: *una battuta di
pesca che si trasforma in una fuga da una tribù di Murloc*. Altri esempi voluti: un drop raro, le cose tipiche
che poi si raccontano tra amici.

Il perché: ricordare e ridere insieme dei momenti di gioco, senza che nessuno debba scriverli a mano.

## Obiettivi

- Ogni membro installa **un addon** e **un programmino** una volta sola; dopo non deve fare nulla.
- Ogni giorno esce **una pagina** di diario con i **3–5 momenti migliori** di tutta la gilda, intrecciati.
- Voce: **narratore epico alla World of Warcraft** (tono dei testi di missione e dei filmati), solenne anche
  per le figuracce: il contrasto è voluto. In italiano.
- Il diario non dipende dal PC di nessuno ed è **gratuito** oltre all'abbonamento Claude dell'autore.
- Addon e programmino pronti **per il lancio** (4 novembre sera), così si registra dal primo giorno. Il bot
  del diario può arrivare qualche giorno dopo: i dati si accumulano e la prima pagina può recuperare.

## Non-obiettivi

- Nessuna lettura della **chat** (privata, di gilda, di gruppo).
- Nessuna distribuzione pubblica (CurseForge ecc.): è solo per questa gilda.
- Nessuna pubblicazione in tempo reale, nessun sito web o dashboard, nessuna statistica.
- Nessun supporto ad altre versioni di WoW (Classic Era, Retail): solo il client Forever.

## Anti-obiettivi (fallimento anche se "funziona")

- I giocatori devono fare passi manuali a ogni sessione.
- L'addon rompe o rallenta l'interfaccia, causa errori Lua o *taint*.
- Il diario è un elenco di fatti invece di un racconto; o è sempre uguale.
- Il diario si inventa fatti che non sono successi (il narratore può colorire, non inventare eventi).

## Vincoli tecnici noti

- **Il client Forever è il client moderno** (game type `camelot`, famiglia mainline, `## Interface: 16001`),
  con le restrizioni di Midnight. Riferimento verificato: il progetto di traduzione dell'autore in
  `C:\Users\old_p\Documents\wow forever traduzione` (`docs/architecture/client-limits.md`,
  `docs/adr/035-ui-errors-frame-surface.md`, `docs/adr/029-camelot-targets-the-mainline-family.md`).
- **`COMBAT_LOG_EVENT_UNFILTERED` è chiuso agli addon** su Forever; molti valori in combattimento sono
  *secret values* (`issecretvalue`): si possono tenere ma non leggere/confrontare. In dungeon/raid/encounter
  anche le righe di sistema e PNG in chat possono essere secret (`SecretInChatMessagingLockdown`).
- **Il file `Logs/WoWCombatLog.txt` resta leggibile da programmi esterni**: è la fonte dei dettagli di
  combattimento (quanti nemici, vita minima, chi ti ha ucciso, cure decisive).
- **Un addon non può usare la rete.** Può solo scrivere le SavedVariables, che il gioco salva su disco a
  logout, `/reload` e uscita (non in caso di crash).
- Da verificare al lancio: nome della cartella del client (in beta `_classic_beta_`); se un addon può
  attivare il registro combattimenti da solo (`LoggingCombat(true)`) e il *advanced combat logging*.
- Il progetto di traduzione è un buon modello di struttura: Lua puro con dipendenze iniettate, test con
  `busted`, `luacheck`, controllo dei secret values tramite `Compat`. Riprendere quell'impostazione.

## Architettura (tre pezzi)

```
 [WoW Forever]                    [PC del giocatore]                 [Discord]                [Cloud Anthropic]
  Addon DiarioGilda  ──SV su disco──▶  Programmino  ──webhook──▶  #diario-servizio  ◀──legge──  Attività programmata
                     WoWCombatLog.txt ─┘ (estrae, sfoltisce,                                  Claude Code (10:00)
                                          comprime, invia)          #diario-gilda  ◀──pubblica──┘
```

### 1. Addon (`DiarioGilda`, Lua, client Forever)

Registra **fatti**, non racconti, in SavedVariables account-wide, una voce per sessione
(personaggio, reame, gilda, inizio/fine, elenco eventi con timestamp locale `time()` e zona/sottozona).

Eventi da registrare (lista approvata):

- **Bottino**: oggetti di qualità rara o superiore ricevuti, con fonte (mob/forziere/pesca) quando nota;
  conteggio dei tentativi ripetuti sullo stesso boss (per "il 40° tentativo").
- **Livelli**: ogni level up, con la zona.
- **Morti**: dove, causa se nota, conteggio per zona. (Caduta/annegamento si ricavano anche dal combat log.)
- **Boss e wipe**: inizio/fine incontro, successo o no, tentativi, membri del gruppo.
- **Gruppi**: con quali membri della gilda si è giocato e per quanto.
- **Missioni**: completate (evidenziare quelle lunghe/note) e abbandonate.
- **Mestieri**: primo oggetto raro creato; pesca (lanci e pescato strano).
- **Duelli**: vinti/persi, contro chi.
- **Soldi**: acquisti grossi (es. prima cavalcatura), spese folli all'asta.
- **Viaggi**: prima volta in una zona; spostamenti lunghi.
- **Combattimento**: solo inizio/fine (`PLAYER_REGEN_DISABLED/ENABLED`) come segnaposto temporale; il
  dettaglio arriva dal combat log.

Regole: mai toccare frame sicuri, mai causare taint; ogni valore che `issecretvalue` segnala viene scartato,
non letto. Le SavedVariables hanno un tetto di dimensione; le sessioni già inviate vengono marcate e potate.
Un comando `/diario` mostra stato e ultima sessione. All'avvio l'addon prova ad attivare il registro
combattimenti; se non può, avvisa il giocatore con un promemoria chiaro (`/combatlog`).

### 2. Programmino (Windows e macOS)

Deve essere **facile per tutti** (richiesta dell'autore): installazione in pochi clic, poi nessun intervento.

- Due versioni dallo stesso codice: **Windows** (un `.exe`, area di notifica, avvio automatico) e **macOS**
  (app nella barra dei menu, avvio al login). Alcuni membri usano il Mac.
- Al primo avvio trova la cartella di WoW (o la chiede), **installa da solo l'addon** in `Interface/AddOns` e
  chiede il nome del giocatore da usare nel diario.
- **Si aggiorna da solo**, addon compreso (richiesta dell'autore): controlla periodicamente le nuove
  versioni, scarica e sostituisce sé stesso; copia la nuova versione dell'addon in `Interface/AddOns` (vale
  dal login o `/reload` successivo). Gli aggiornamenti devono essere verificati (firma o hash) prima di
  essere applicati, e un aggiornamento fallito deve lasciare funzionante la versione precedente.
- Il meccanismo di aggiornamento deve essere **pronto per il lancio**: le correzioni della prima settimana
  arrivano così.
- Quando il file SavedVariables dell'addon cambia (logout/reload/uscita), aspetta che si stabilizzi, legge le
  sessioni nuove, estrae da `WoWCombatLog.txt` la finestra temporale di quelle sessioni e la **riduce a
  episodi** di combattimento che riguardano il giocatore (nemici coinvolti, vita minima se disponibile,
  colpi più grossi, colpo mortale, cure ricevute vicino alla morte, danni ambientali).
- Invia un file `.json.gz` per sessione al canale `#diario-servizio` tramite **webhook** (nessun bot sui PC
  dei giocatori). Deve stare sotto il limite allegati di Discord.
- Tiene traccia di cosa ha già inviato; se Discord non risponde ritenta più tardi; non perde dati.
- L'URL del webhook è nel file di configurazione dello zip: accettato, sono amici.
- Riserva manuale: se il programmino non funziona, il giocatore può trascinare il file SavedVariables nel
  canale di servizio e il bot lo accetta comunque (senza dettagli di combattimento).

### 3. Bot del diario (attività programmata di Claude Code nel cloud)

- Gira **ogni giorno alle 10:00 ora italiana** come *routine* programmata di Claude Code, con l'abbonamento
  dell'autore. Il PC dell'autore può restare spento.
- La **giornata di gioco** va dalle 06:00 alle 06:00 (Europe/Rome). Una sessione appartiene alla giornata in
  cui è iniziata; le sessioni arrivate in ritardo finiscono nella pagina successiva.
- Legge da `#diario-servizio` i file della giornata (Discord REST con un bot token), li unisce, riconosce gli
  **episodi condivisi** (più membri nello stesso posto/momento/incontro diventano un solo racconto), sceglie i
  3–5 momenti migliori, scrive la pagina e la pubblica in `#diario-gilda`.
- **Memoria**: un file di stato (conteggi tentativi, ricorrenze tipo "Gino e i Murloc", numero del giorno)
  allegato in `#diario-servizio` ogni giorno; il bot legge l'ultimo. Nessun altro archivio.
- Se un giorno non ci sono dati, non pubblica nulla. Se il bot parte qualche giorno dopo il lancio, la prima
  esecuzione pubblica **una sola pagina di recupero** ("Le Cronache dei primi giorni"), poi una al giorno.
- **Personaggi**: per ora nessuno ha alt, quindi si registra ogni sessione del personaggio del giocatore,
  dentro o fuori gilda (il primo giorno la gilda in gioco potrebbe non esistere). Il personaggio è associato
  al nome scelto nel programmino di quel PC. Gli alt sono un tema futuro.
- Segreti (bot token) nelle impostazioni dell'ambiente della routine, mai nel repository.
- **Da verificare per primo**: che l'ambiente cloud della routine possa raggiungere `discord.com` e tenere
  segreti. Se no, **piano B: GitHub Actions** in un repository **privato**, con `claude setup-token` per
  usare l'abbonamento. Il resto non cambia.

## La pagina di diario

Esempio di tono approvato dall'autore:

> **Cronache della Gilda — Giorno 12, Era della Rinascita**
>
> Narrano le genti di Westfall che, quando il sole calava sulle acque del Mare Velato, Gino il Pescatore
> gettò la sua lenza con la calma di chi ha visto mille tramonti. Non sapeva, il prode, che sotto quella
> superficie quieta dormiva un'intera stirpe di Murloc. […] Ottanta passi di corsa disperata, e la tribù
> rimase sulla riva a gorgogliare il suo nome.

- Una pagina al giorno, 3–5 momenti, racconto intrecciato (non un elenco), con titolo e numero del giorno.
- Deve stare nei limiti di Discord (embed fino a 4096 caratteri di descrizione, o più messaggi).
- Usa i nomi dei giocatori come scelti nel programmino; i nomi di mob, zone e oggetti come li dà il gioco.
- Fedeltà: ogni momento deve corrispondere a eventi nei dati.

## Discord (lo prepara l'autore, con una guida passo passo)

- Canale `#diario-gilda` (lettura per tutti) e canale `#diario-servizio` (visibile ai membri ma silenziato,
  così chi usa la riserva manuale può caricarci il file).
- Un webhook su `#diario-servizio` per i programmini.
- Un'applicazione bot con i soli permessi di leggere `#diario-servizio` e scrivere in entrambi i canali.

## Distribuzione

Un pacchetto per Windows e uno per macOS (il programmino installa l'addon) con una guida in italiano di due
minuti, linkati su Discord. Le versioni successive arrivano con l'aggiornamento automatico. La
privacy: i compagni sanno che vengono raccolti eventi di gioco (non la chat) con nome, posizione e orari.

## Verifica

- Addon: logica pura testata con `busted` su eventi simulati; `luacheck`; prova in gioco al lancio.
- Programmino: test con un `WoWCombatLog.txt` e SavedVariables di esempio; prova di invio su un canale di test;
  prova di aggiornamento da una versione all'altra (anche fallito) su Windows e macOS.
- Bot: modalità prova che pubblica in un canale di test, su dati di esempio (inclusa la fuga dai Murloc).

## Decisioni prese da Claude (da controllare)

- Linguaggio del programmino: **Go** (un solo eseguibile senza installazioni, compila per Windows e macOS
  dallo stesso codice). Il bot usa script semplici chiamati da Claude Code.
- Aggiornamenti pubblicati come **GitHub Releases** di un repository con il solo codice (nessun dato dei
  giocatori), il canale che il progetto di traduzione usa già.
- macOS **senza firma Apple** (costerebbe 99 $/anno): al primo avvio serve una volta "tasto destro → Apri",
  spiegato nella guida. Windows potrebbe mostrare l'avviso SmartScreen la prima volta: idem.
- Pagina di recupero unica se il bot parte dopo il lancio; canale di servizio visibile ma silenziato.
- Giornata 06:00–06:00, pubblicazione alle 10:00, una sessione appartiene al giorno in cui è iniziata.
- Episodi di combattimento ridotti **sul PC** (non si manda il combat log intero).
- Stato del bot conservato come allegato su Discord.
- Nomi di gioco lasciati come li dà il client; l'uso del glossario del progetto di traduzione è un
  miglioramento futuro.
- Sessioni perse in caso di crash del gioco: accettato.

## Lasciato al costruttore

Struttura dei file, formato esatto degli eventi e dei file inviati, euristiche per riconoscere gli episodi
di combattimento e quelli condivisi, criteri di "momento migliore", testo del prompt del narratore.
