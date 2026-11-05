# Missioni raccontate, bilancio della giornata e livelli bassi — design

Data: 2026-11-05 (giorno dopo il lancio di WoW Forever). Autore: oLd_pZ con Claude.

## Perché

La prima pagina vera del Cronista (Giorno 1, 5/11) ha raccontato solo due morti e una prima visita, anche se
la serata aveva 6 missioni completate, 106 combattimenti e 5 livelli. Le regole dei momenti sono pensate per
eventi grandi (boss, epici, livelli tondi): una serata ai livelli bassi, o da soli, resta quasi vuota.

L'utente vuole soprattutto **rivivere la storia del gioco attraverso le missioni fatte davvero**, e che ogni
serata lasci una traccia nella pagina anche se non è successo niente di clamoroso.

## Decisioni prese con l'utente

- **Una sola pagina comune per la gilda**, come oggi. Niente cronache personali o discussioni separate
  (scartate: troppo complicate).
- La pagina ha due parti:
  1. **Racconto**: 3–5 momenti come oggi, ma tra i candidati entrano anche le **missioni completate**; il
     Cronista può raccontarne una o due delle più belle.
  2. **Bilancio finale**: poche righe per ogni giocatore della giornata (livelli, missioni, nemici abbattuti,
     morti, luoghi).
- **Livelli bassi**: contano tutti i livelli fino al 10 (oggi solo 10, 20, … 60); conta anche il bottino
  verde nei primi livelli (oggi solo blu o meglio).
- **Trama vera**: l'addon comincia a salvare i testi delle missioni; finché non ci sono (sessioni vecchie,
  amici non aggiornati) il Cronista racconta la missione dal titolo e da quello che il giocatore ha fatto
  davvero. Questa scelta (entrambe le cose) l'ha delegata l'utente a Claude.

## Obiettivi

1. Missioni completate come momenti candidati, con fatti veri: titolo, luogo, esperienza, nemici abbattuti
   collegati alla missione, e (se presenti) i testi della missione.
2. Bilancio della giornata per giocatore, **calcolato dal codice** (numeri esatti, non scritti dal modello),
   in fondo alla pagina.
3. Livelli fino a 10 e bottino verde ai livelli bassi come momenti minori.
4. Addon: salvare descrizione, obiettivi e testo di consegna della missione alla consegna.

## Non obiettivi

- Cronache personali per giocatore, discussioni Discord, nuovi permessi del bot.
- Raccontare *tutte* le missioni nel corpo della pagina: il corpo resta di 3–5 momenti.
- Recuperare i testi delle missioni già completate (le serate passate restano senza trama).
- Cercare i testi delle missioni sul web: solo dati del gioco del giocatore.

## Anti-obiettivi (sarebbe un fallimento anche se funziona)

- Una pagina che diventa un elenco noioso di missioni o un muro di testo quando giocano in tanti.
- Numeri del bilancio inventati o sbagliati dal modello.
- Trame inventate: se il testo della missione non c'è, il Cronista non deve "ricordare" la storia della
  missione da quello che sa del gioco. Solo titolo e fatti.
- Il file del diario (`DiarioGilda.lua`) che cresce tanto da superare i limiti (`maxSVSize` nel programmino,
  dimensione dei pacchetti su Discord).
- Testi del gioco usati per far eseguire istruzioni al Cronista (sono dati, come tutto il resto).

## Vincoli

- Il Cronista gira ogni mattina alle 07:15 e deve continuare a funzionare con pacchetti vecchi (0.1.0–0.1.3,
  senza testi) e nuovi, mescolati nella stessa giornata.
- Gli amici aggiornano programmino e addon in automatico (entro 6 ore); i pacchetti già inviati non cambiano.
- Regola esistente: **solo fatti presenti nei dati**; nomi di mob, zone, missioni e oggetti come li scrive il
  client (inglese), non tradotti.
- Il client di Forever è in inglese; la pagina è in italiano.
- Embed Discord: 4096 caratteri per descrizione, 6000 per messaggio, 10 embed per messaggio (già gestiti da
  `pagina.embed`, che spezza in più messaggi).

## Come (le parti decise)

### Addon (nuova versione 0.2.0)

- Alla consegna (`QUEST_COMPLETE`, quando la finestra di consegna è aperta e la missione è ancora nel
  registro) l'addon legge e ricorda per quell'id:
  - **descrizione** e **obiettivi** della missione dal registro missioni (API classiche:
    `GetQuestLogQuestText` dopo aver selezionato la voce, oppure le equivalenti `C_QuestLog`);
  - **testo di consegna** (`GetRewardText`).
- A `QUEST_TURNED_IN` l'evento `quest` `action="done"` già esistente riceve i campi in più `desc`, `obj`,
  `fine` (stringhe), se letti per quell'id. Ogni testo è **troncato** (proposta: 600 caratteri desc,
  300 obj, 400 fine) per tenere il file piccolo.
- Ogni chiamata passa da `DG.Safe` / `pcall` come il resto: se l'API non esiste su Forever, i campi
  semplicemente mancano.
- `MIN_QUALITY` diventa dinamico: oggetti **verdi** (qualità 2) registrati solo se il livello del personaggio
  è **sotto 20**; dal 20 in su resta da blu in su.
- Da verificare in gioco (si aggiunge una voce a `docs/verifica-al-lancio.md`): che `desc/obj/fine`
  compaiano davvero nel file su Forever.

### Programmino

- Nessuna logica nuova: porta i campi in più così come sono (verificare che `addondata`/`payload` non
  scartino campi sconosciuti degli eventi; oggi `Fields` tiene tutto).
- Esce come 0.2.0 insieme all'addon (stessa versione, come sempre).

### Cronista (bot)

**Missioni come momenti** (`momenti.py`):
- Ogni `quest` `done` diventa un candidato `missione` con fatti: `missione` (titolo), `luogo`, `xp`, e se ci
  sono `trama` (desc), `obiettivi`, `consegna` (fine) — testi passati da `pulito`/troncati come gli altri.
- **Nemici collegati**: le uccisioni (`kills` degli episodi di combattimento) del giocatore prima della
  consegna, così:
  - se ci sono gli obiettivi e citano un nemico ucciso (nome del mob contenuto nel testo degli obiettivi,
    senza maiuscole, anche al plurale con "s"), si contano quelle uccisioni in una finestra prima della
    consegna (vedi "Lasciato al costruttore");
  - altrimenti: uccisioni nella stessa sottozona nei 30 minuti prima della consegna, con
    `nemici_presunti: sì` (il Cronista scrive "probabilmente").
- Punti: base bassa (proposta 12), più alta se ha la trama (proposta 20), così una missione con storia
  batte una prima visita ma non una morte buffa o un boss. Al massimo **2 missioni per giocatore** tra i
  candidati della giornata (le altre restano nel bilancio), scelte per punti poi per esperienza.

**Livelli bassi**: per ogni giocatore, un solo candidato `livelli` per giornata con i livelli sotto il 10
presi ("dal 1 al 6"), punti 15. I livelli tondi restano candidati separati come oggi.

**Bottino verde**: un verde diventa candidato `drop` (punti 8) solo se il livello del personaggio al momento
del drop è sotto 15; al massimo uno per giocatore al giorno (il primo).

**Bilancio** (nuovo, deterministico):
- `prepara` calcola per ogni giocatore della giornata: personaggio, livello iniziale → finale, missioni
  completate (numero + titoli, al massimo 8, poi "e altre N"), nemici abbattuti (somma `kills` degli
  episodi; se non ci sono episodi, si omette), morti, luoghi (sottozone/zone distinte, al massimo 5).
- Lo scrive in `bot/lavoro/bilancio-AAAA-MM-GG.md` e lo cita nel riassunto per il Cronista (solo per
  contesto: non va copiato nel racconto).
- `pubblica` **aggiunge da sé** il bilancio in fondo alla pagina di quella giornata, dopo il racconto,
  sotto un'intestazione fissa ("**Bilancio della giornata**"); il modello non lo scrive. Se la pagina è
  quella di recupero (più giornate), un bilancio unico sommato per giocatore.
- Formato di una riga (esempio):
  `**Sciatalgia** — livello 1 → 6 · 6 missioni (Rude Awakening, The Mindless Ones, …) · 40 nemici · 2 morti · Deathknell, Shadow Grave, Solliden Farmstead`

**Istruzioni** (`bot/ISTRUZIONI.md`):
- Le missioni sono momenti come gli altri; se c'è la trama, riassumerla in italiano in 1–2 frasi tenendo i
  nomi propri come sono; senza trama, raccontare solo titolo, luogo e nemici abbattuti, **senza
  aggiungere la storia della missione** da conoscenze proprie.
- Il bilancio lo aggiunge `pubblica`: non scriverlo, non ripeterne i numeri come elenco.
- Varietà: non più di una missione per giocatore nel racconto, se ci sono altri momenti.

## Scelte prese da Claude (dopo la revisione)

- **Bilancio per personaggio, raggruppato per giocatore.** Dalla 0.1.1 arriva solo il personaggio
  principale, quindi di solito è una riga per giocatore; se nella giornata ci sono più personaggi dello stesso
  giocatore (pacchetti vecchi o `.lua` a mano), una riga per personaggio. I livelli si calcolano per
  personaggio (livello della sessione + eventi `level`).
- **Sessioni in ritardo**: non entrano nei numeri della giornata. Hanno una riga a parte nel bilancio,
  marcata con la giornata vera ("notizie dal 4 novembre"), con gli stessi campi.
- **Dati veri per i test**: il file `DiarioGilda.lua` e il registro combattimenti del 4/11 sono sul PC
  dell'utente (`D:\WoW\World of Warcraft\_forever_\WTF\Account\…\SavedVariables\DiarioGilda.lua` e
  `…\Logs\WoWCombatLog-110526_002041.txt`). Se ne ricava una fixture con i nomi degli altri giocatori tolti.
- **Missioni consegnate in gruppo**: si uniscono con `_unisci` come gli altri momenti (un solo momento con
  più giocatori). Il limite di 2 missioni per giocatore si applica **dopo** l'unione, contando ogni momento
  unito una volta per ciascun giocatore coinvolto.
- Il livello del personaggio al momento di un drop si ricava dagli eventi `level` precedenti (o dal livello
  della sessione).

## Lasciato al costruttore

- La finestra esatta per collegare le uccisioni agli obiettivi (es. dall'ultima consegna di quel giocatore
  nella stessa zona, o un limite fisso di 60 minuti): scegliere la regola più semplice che dà numeri
  credibili sui dati veri del 4/11 (6 missioni a Deathknell).
- I valori esatti di punti e limiti di caratteri (quelli sopra sono proposte).
- Come leggere i testi dal registro missioni su Forever (quale API esiste davvero): scrivere il codice con
  fallback e lasciare la verifica alla voce nuova della lista di verifica.
- Struttura dei test (pytest/unittest come oggi in `bot/tests`, busted per l'addon, Go per il programmino).

## Ordine consigliato

1. Bot: bilancio + missioni (senza testi) + livelli bassi + verdi → utile da subito con i dati che ci sono.
2. Addon + programmino 0.2.0: testi delle missioni e verdi sotto il 20.
3. Bot: uso dei testi quando presenti (può arrivare insieme al punto 1, i campi mancanti sono già gestiti).
