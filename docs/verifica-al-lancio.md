# Verifica al lancio (5 novembre)

Lista da spuntare in gioco su WoW Forever con l'addon installato. Ogni voce dice cosa fare e cosa fare se fallisce.
Prima di iniziare scrivi `/console scriptErrors 1` (vedi voce 14).

## Installazione

- [ ] **1. Cartella del client e percorso di SavedVariables.** Trova la cartella del client Forever (quella con `Interface/AddOns`)
  e annota il percorso completo. Copia `DiarioGilda/` (dallo zip `dist/DiarioGilda-<versione>.zip`) in `Interface/AddOns/`.
  Il file dei dati sarà `WTF/Account/<ACCOUNT>/SavedVariables/DiarioGilda.lua`. Annota il nome della cartella del client e `<ACCOUNT>`.
  - Se fallisce: l'addon non compare nell'elenco al login. Controlla che la cartella si chiami esattamente `DiarioGilda`
    (non `DiarioGilda/DiarioGilda`) e che il TOC abbia `## Interface: 16001`; se Forever usa un altro numero di interfaccia,
    cambia il TOC. Se il percorso di WTF è diverso, aggiornalo nel Piano 2.

- [ ] **2. `/diario` dopo il login.** Scrivi `/diario`: deve stampare una riga con personaggio, versione (0.1.0) e numero di eventi.
  - Se fallisce (nessuna risposta): l'addon non è stato caricato o è andato in errore al caricamento. Controlla l'elenco addon
    e gli errori Lua (voce 14). Un errore di sintassi indica il file da correggere.

## Registro combattimenti

- [ ] **3. Un addon può chiamare `LoggingCombat(true)`.** Dopo il login deve comparire il file `Logs/WoWCombatLog*.txt`
  (in Forever il nome può includere data e ora). Fai un combattimento e controlla che il file cresca.
  - Se fallisce: l'addon stampa "il registro dei combattimenti è spento" e scrive un evento `note`. Scrivi `/combatlog` a mano a
    ogni sessione e verifica che il file nasca. Se `/combatlog` funziona ma la chiamata da addon no, lascia la nota nel diario e
    ricorda agli utenti di accenderlo. Se anche `/combatlog` non produce il file, il Piano 2 non potrà usare il registro:
    segnalalo, la parte "dettagli di combattimento" va ridimensionata.

- [ ] **4. Campione reale di `WoWCombatLog*.txt` (per il Piano 2).** Dopo un combattimento con un mob, un lancio, un danno
  subito e una morte, copia il file in un posto sicuro. Servono le righe `SPELL_DAMAGE`, `SWING_DAMAGE`, `UNIT_DIED`,
  `ENCOUNTER_START/END`, `ZONE_CHANGE`, se presenti. Serve a confermare la posizione dei campi su Forever.
  - Se fallisce: se il file è vuoto o ha un formato diverso, le posizioni dei campi assunte dal Piano 2 vanno corrette sul campione
    reale prima di scrivere il parser. Non scrivere il parser del registro senza questo file.

## Dati registrati

- [ ] **5. Id della pesca con `/diario debug`.** Scrivi `/diario debug` e lancia la canna: la riga stampata mostra spellId e nome.
  Confronta con `FISHING_SPELLS` in `addon/DiarioGilda/Core/Const.lua` (7620, 7731, 7732, 18248, 131474, 131490).
  - Se fallisce (l'id non è in lista): aggiungi l'id letto a `FISHING_SPELLS` e rigenera lo zip. Senza questo la pesca non
    viene registrata come `fishing`.

- [ ] **6. `LOOT_READY` scatta due volte per finestra di bottino?** Pesca 5-10 pesci e a fine serata (dopo `/reload`) guarda nel
  file l'array `catches` dell'evento `fishing`: se ogni pesce compare due volte di fila, l'evento arriva doppio.
  - Se fallisce (catture doppie): in `Main.lua` ignora un secondo `LOOT_READY` identico entro 1 secondo (o confronta con le
    catture già registrate dello stesso lancio). Il bottino raro non è toccato: nasce da `CHAT_MSG_LOOT`.

- [ ] **7. Un drop raro, una morte, un level up, un cambio zona, un gruppo e un boss finiscono nel file.** Gioca fino a ottenere
  un oggetto blu o migliore, morire una volta, salire di livello, cambiare zona, entrare in gruppo e (se possibile) uccidere
  un boss di dungeon. Poi `/reload` e apri `SavedVariables/DiarioGilda.lua` (le SavedVariables si scrivono su `/reload` e
  logout): devono esserci eventi `loot`, `death`, `level`, `zone`, `group`, `encounter`.
  - Se fallisce per un tipo: quell'evento non arriva. Guarda `errors` nel file (voce 14), poi verifica il nome dell'evento nel
    client Forever (può differire da quello retail). Annota quale manca.

- [ ] **8. `/reload` a metà serata continua la stessa sessione.** Dopo `/reload` il file ha ancora una sola sessione per la serata
  (stesso `id`), senza `stop`, con i nuovi eventi accodati.
  - Se fallisce (due sessioni): `PLAYER_ENTERING_WORLD` non arriva con `isReload` vero. Annota gli argomenti reali e correggi
    `Main.lua`.

## API di gioco da verificare

- [ ] **9. `AbandonQuest` globale e `C_QuestLog.GetAbandonQuest()` nel post-hook.** Abbandona una missione di prova (poi
  riprendila). Nel file deve esserci un evento `quest` con `action = "abandon"`, `id` e `title`. A mano: `/dump AbandonQuest`
  e `/dump C_QuestLog.GetAbandonQuest()` (quest'ultimo mentre il popup di conferma è aperto).
  - Se fallisce: se `AbandonQuest` non è una funzione l'hook non viene installato (nessun errore, solo nessun evento `abandon`);
    se l'id è `nil` nel post-hook, la conferma lo ha già azzerato: salvalo quando si apre il popup di conferma, oppure rinuncia
    agli abbandoni nella v1.

- [ ] **10. `CHAT_MSG_LOOT` e `CHAT_MSG_SYSTEM` in dungeon (valori secret?).** In un dungeon ottieni un drop e un messaggio di
  sistema, poi controlla `errors` nel file e che il drop sia registrato.
  - Se fallisce (drop mancante in istanza): il messaggio arriva secret e viene scartato di proposito. Accetta la perdita oppure
    ricostruisci il drop da `LOOT_READY` + `GetLootSlotInfo`. Se invece compare un errore Lua è un bug: copia l'errore.

- [ ] **11. Nomi del gruppo e del boss in dungeon.** Entra in un dungeon con un gruppo: l'evento `group` deve avere i nomi
  (oppure mantenere l'ultimo gruppo noto se qualcuno è secret); l'evento `encounter` deve avere `name`.
  - Se fallisce: se `members` è sempre vuoto in istanza i nomi sono secret e il Piano 2 deve vivere con `group` assente. Se
    `encounter` non ha `name` va bene: restano `id`, `ok` e i tempi.

## Pulizia e stabilità

- [ ] **12. Crash simulato.** Chiudi il client dal Task Manager e riaccedi: la sessione interrotta nel file deve avere `stop`
  uguale all'ultimo evento, e deve esserci una nuova sessione aperta.
  - Se fallisce: dopo un kill le SavedVariables non vengono scritte, quindi si perde tutto dall'ultimo `/reload` o logout. È
    il comportamento del client: fai un `/reload` ogni tanto nelle serate lunghe. Se invece il file c'è ma la sessione resta
    senza `stop`, è un bug in `Journal.begin`.

- [ ] **13. Dimensione del file dopo una serata.** Dopo 2-3 ore il file `DiarioGilda.lua` deve restare sotto qualche centinaio
  di KB. Il tetto è 5000 eventi per sessione (`dropped` conta il resto).
  - Se fallisce (file enorme o `dropped` alto): guarda quale tipo `k` domina (di solito `zone` o `combat`) e riduci la
    frequenza di quell'evento in `Recorder.lua`.

- [ ] **14. Nessun errore Lua.** Con `/console scriptErrors 1` non deve apparire nessuna finestra di errore in tutta la serata, e
  `errors` nel file deve essere vuoto o spiegabile.
  - Se fallisce: copia il testo dell'errore (o la riga di `errors` nel file) con nome del file e riga. Ogni handler è protetto
    da `pcall`, quindi l'errore finisce in `errors` e non rompe il gioco. Correggi dal messaggio, non a occhio.

## Esito

Annota qui la data e per ogni voce V (ok) o X (fallita, con cosa hai trovato) prima di passare al Piano 2.
