# Diario di Gilda: installazione (due minuti)

Il Diario di Gilda è un piccolo programma che, quando chiudi World of Warcraft, manda al canale della gilda il
racconto della tua sessione di gioco. Installa da solo l'addon in WoW e si aggiorna da solo.

## 1. Scarica lo zip giusto

Dalla pagina delle release (<https://github.com/oLdpZ/diario-gilda/releases/latest>) scarica:

- **Windows:** `DiarioGilda-windows-amd64.zip`
- **Mac:** `DiarioGilda-macos.zip`

I file `.sig` accanto agli zip servono al programma per controllare gli aggiornamenti: non ti servono.

## 2a. Windows

1. Tasto destro sullo zip → **Estrai tutto…** → **Sfoglia…** → scegli **Documenti**, crea la cartella
   `DiarioGilda` e confermala, così il percorso è `Documenti\DiarioGilda` → **Estrai**.
   Dentro ci sono `DiarioGilda.exe` e un file `VERSION`: il file `VERSION` puoi ignorarlo, ma lascialo lì.
   Se i tuoi Documenti sono sincronizzati con OneDrive e gli aggiornamenti non riescono, sposta la cartella in
   un posto non sincronizzato (per esempio `C:\DiarioGilda`) e riavvia il programma da lì.
2. Doppio clic su `DiarioGilda.exe`.
3. Se compare la finestra blu di Windows ("Windows ha protetto il PC", SmartScreen): clicca
   **Ulteriori informazioni**, poi **Esegui comunque**. Succede solo la prima volta: il programma non ha un
   certificato a pagamento, ma gli aggiornamenti sono firmati e controllati.
   Se invece Windows dice che **Smart App Control** ha bloccato l'app e non offre "Esegui comunque", quel blocco
   non si può superare per una singola app: avvisa l'autore (l'alternativa è spegnere Smart App Control in
   Sicurezza di Windows → Controllo delle app e del browser, ma una volta spento non si riaccende senza
   reinstallare Windows).

## 2b. Mac

1. Doppio clic sullo zip in Download: compare `DiarioGilda.app`. Se Safari ha già estratto lo zip da solo, in
   Download trovi direttamente `DiarioGilda.app` (o una cartella `DiarioGilda` che lo contiene): usa quello.
2. **Trascina `DiarioGilda.app` nella cartella Applicazioni.** Deve restare lì: se lo apri direttamente da
   Download o da dentro lo zip, macOS lo esegue da una copia temporanea di sola lettura ("App Translocation") e il
   programma **non può aggiornarsi**. Se ti avvisa che l'aggiornamento non è riuscito, controlla questo.
3. La prima volta macOS lo blocca perché l'autore non è uno sviluppatore registrato da Apple:
   - fai doppio clic su `DiarioGilda.app` in Applicazioni; al messaggio di blocco clicca **Fine** (o **OK**);
   - apri **Impostazioni di Sistema → Privacy e sicurezza**, scorri in fondo fino al messaggio su
     "DiarioGilda" e clicca **Apri comunque**; conferma con la password del Mac (o Touch ID) e poi **Apri**.

   Su macOS più vecchi (fino a macOS 14 Sonoma) basta anche: **tasto destro** (o ctrl+clic) su
   `DiarioGilda.app` → **Apri** → **Apri**. Dalle volte successive parte da solo.

## 3. Primo avvio

- Il programma cerca World of Warcraft. Se non lo trova (per esempio è su un altro disco), ti chiede di indicare la
  cartella dove è installato.
- Ti chiede con che nome vuoi comparire nel diario: scrivi il nome con cui ti conosce la gilda.
- Fatto: installa l'addon **DiarioGilda** in WoW e da quel momento lavora da solo. Lo trovi come icona vicino
  all'orologio (Windows: in basso a destra, a volte dentro la freccetta `^`; Mac: in alto a destra nella barra
  dei menu). Parte da solo ogni volta che accedi al computer.

Dal menu dell'icona puoi: **Invia ora**, **Cambia nome…**, **Apri cartella dei registri** (se qualcosa non va,
manda all'autore il file `diario.log` che trovi lì), **Esci**.

## 4. Controlla in gioco

Entra in gioco e scrivi `/diario` in chat: l'addon risponde con la sua versione, il personaggio e quanti eventi
ha registrato. Se il programma ti avvisa "Addon aggiornato: scrivi /reload in gioco", scrivi `/reload`.

Consiglio: lascia acceso il registro combattimenti (`/combatlog`) per avere anche i dettagli delle battaglie.

## Cosa viene raccolto

- **Eventi di gioco del tuo personaggio**, ciascuno con orario e posizione (zona, sottozona, coordinate):
  cambi di zona, livelli, morti, missioni consegnate, bottino e oggetti creati di qualità rara o migliore,
  pesca, incontri con i boss, duelli, spese grosse, composizione del gruppo (nomi dei compagni).
- **Dati del personaggio**: nome, reame, gilda, classe, razza, livello.
- **Un riassunto dei combattimenti** dal registro combattimenti di WoW (`Logs/WoWCombatLog*.txt`): durata, nemici,
  danni subiti, colpi più forti, chi ti ha curato, chi ti ha ucciso.
- Il nome che hai scelto per il diario e la versione del programma.

Tutto questo parte solo verso il canale Discord della gilda, una volta per sessione.

## Cosa NON viene mai raccolto

- **La chat**: nessun messaggio, sussurro, canale o chat di gilda.
- Password, dati dell'account Battle.net, email o altri dati del computer.
- Il programma non raccoglie il tuo indirizzo IP: come per ogni sito, Discord e GitHub vedono da dove arriva la
  connessione, ma il programma non lo registra e non lo mette nei file inviati.
- File fuori dalla cartella di WoW. Il programma legge solo i dati dell'addon e il registro combattimenti, e in WoW
  scrive solo nella cartella `Interface/AddOns/DiarioGilda` (durante un aggiornamento dell'addon possono comparire
  per un attimo anche `DiarioGilda.new` e `DiarioGilda.old`, che spariscono da sole).

## Se il programma non funziona: invio a mano

L'addon salva comunque tutto in WoW. Dopo essere uscito dal gioco:

1. Apri la cartella di WoW, poi `WTF/Account/<ACCOUNT>/SavedVariables/` (`<ACCOUNT>` è il nome della cartella del
   tuo account).
2. Trascina il file `DiarioGilda.lua` nel canale Discord **#diario-servizio** della gilda.
