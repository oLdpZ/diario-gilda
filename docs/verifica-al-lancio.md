# Verifica al lancio (5 novembre)

Lista da spuntare in gioco su WoW Forever con l'addon installato. Ogni voce dice cosa fare e cosa fare se fallisce.
Prima di iniziare scrivi `/console scriptErrors 1` (vedi voce 15).

## Installazione

- [ ] **1. Cartella del client e percorso di SavedVariables.** Trova la cartella del client Forever (quella con `Interface/AddOns`)
  e annota il percorso completo. Copia `DiarioGilda/` (dallo zip `dist/DiarioGilda-<versione>.zip`) in `Interface/AddOns/`.
  Il file dei dati sarà `WTF/Account/<ACCOUNT>/SavedVariables/DiarioGilda.lua`. Annota il nome della cartella del client e `<ACCOUNT>`.
  - Se fallisce: l'addon non compare nell'elenco al login. Controlla che la cartella si chiami esattamente `DiarioGilda`
    (non `DiarioGilda/DiarioGilda`) e che il TOC abbia `## Interface: 16001`; se Forever usa un altro numero di interfaccia,
    cambia il TOC. Se il percorso di WTF è diverso, aggiornalo nel Piano 2.

- [ ] **2. `/diario` dopo il login.** Scrivi `/diario`: deve stampare tre righe: versione (0.1.0) e personaggio; eventi della
  sessione corrente e stato del registro combattimenti; sessioni salvate ed errori.
  - Se fallisce (nessuna risposta): l'addon non è stato caricato o è andato in errore al caricamento. Controlla l'elenco addon
    e gli errori Lua (voce 15). Un errore di sintassi indica il file da correggere. Se "errori" è maggiore di 0 subito dopo
    il login, apri il file (dopo `/reload`) e leggi `errors`: una voce `Main.RegisterEvent <NOME>` vuol dire che il client
    non conosce quell'evento (l'addon funziona lo stesso, ma quel tipo di evento manca).

## Registro combattimenti

- [ ] **3. Un addon può chiamare `LoggingCombat(true)`.** Dopo il login deve comparire il file `Logs/WoWCombatLog*.txt`
  (in Forever il nome può includere data e ora). Fai un combattimento e controlla che il file cresca.
  - Se fallisce: l'addon stampa "il registro dei combattimenti è spento" e scrive un evento `note`. Scrivi `/combatlog` a mano a
    ogni sessione e verifica che il file nasca. Se `/combatlog` funziona ma la chiamata da addon no, lascia la nota nel diario e
    ricorda agli utenti di accenderlo. Se anche `/combatlog` non produce il file, il Piano 2 non potrà usare il registro:
    segnalalo, la parte "dettagli di combattimento" va ridimensionata.
  - Se al login compare un popup di "azione bloccata" (AddOn ha tentato un'azione bloccata) che nomina DiarioGilda: su Forever
    la chiamata è protetta. Togli la chiamata a `LoggingCombat(true)` in `Main.lua` (`PLAYER_ENTERING_WORLD`), lascia solo il
    controllo con il messaggio e accendi il registro a mano con `/combatlog`.

- [ ] **4. Registro di combattimento avanzato acceso.** In Opzioni → Rete spunta "Registro di combattimento avanzato", oppure
  controlla con `/dump C_CVar.GetCVar("advancedCombatLogging")` (deve stampare `"1"`; a mano si accende con
  `/console advancedCombatLogging 1`). Va acceso prima di prendere il campione della voce 5.
  - Se fallisce (opzione assente o il valore resta `"0"`): annotalo. Senza registro avanzato le righe non hanno posizione e
    vita delle unità: il Piano 2 deve funzionare senza quei campi.

- [ ] **5. Campione reale di `WoWCombatLog*.txt` (per il Piano 2).** Con il registro avanzato acceso (voce 4),
  dopo un combattimento con un mob, un lancio, un danno subito e una morte, copia il file in un posto sicuro. Servono le righe `SPELL_DAMAGE`, `SWING_DAMAGE`, `UNIT_DIED`,
  `ENCOUNTER_START/END`, `ZONE_CHANGE`, se presenti. Serve a confermare la posizione dei campi su Forever.
  - Se fallisce: se il file è vuoto o ha un formato diverso, le posizioni dei campi assunte dal Piano 2 vanno corrette sul campione
    reale prima di scrivere il parser. Non scrivere il parser del registro senza questo file.

## Dati registrati

- [ ] **6. Id della pesca con `/diario debug`.** Scrivi `/diario debug` e lancia la canna: la riga stampata mostra spellId e nome.
  Confronta con `FISHING_SPELLS` in `addon/DiarioGilda/Core/Const.lua` (7620, 7731, 7732, 18248, 131474, 131490).
  - Se fallisce (l'id non è in lista): aggiungi l'id letto a `FISHING_SPELLS` e rigenera lo zip. Senza questo la pesca non
    viene registrata come `fishing`.

- [ ] **7. Catture della pesca e fonte del bottino.** Pesca 5-10 pesci e a fine serata (dopo `/reload`) guarda nel file l'array
  `catches` dell'evento `fishing`: ogni pesce deve comparire una volta sola. `Main.lua` usa solo il primo `LOOT_READY` di
  ogni finestra di bottino (fino a `LOOT_CLOSED`).
  - Se fallisce (catture doppie): `LOOT_CLOSED` arriva fra due `LOOT_READY` della stessa finestra; annota l'ordine degli eventi
    (con `/etrace`) e correggi il controllo in `Main.lua`. Se invece dopo il primo pesce non si registra più nulla,
    `LOOT_CLOSED` non arriva: stesso controllo, al contrario.
  - Con il bottino automatico acceso uccidi un mob che lascia un oggetto blu: l'evento `loot` deve avere `src` = nome del mob.
    Se `src` manca, il messaggio di bottino arriva dopo `LOOT_CLOSED` (che azzera la fonte): in `Recorder.onLootClosed` tieni
    la fonte ancora per 1-2 secondi invece di azzerarla subito.

- [ ] **8. Un drop raro, una morte, un level up, un cambio zona, un gruppo e un boss finiscono nel file.** Gioca fino a ottenere
  un oggetto blu o migliore, morire una volta, salire di livello, cambiare zona, entrare in gruppo e (se possibile) uccidere
  un boss di dungeon. Poi `/reload` e apri `SavedVariables/DiarioGilda.lua` (le SavedVariables si scrivono su `/reload` e
  logout): devono esserci eventi `loot`, `death`, `level`, `zone`, `group`, `encounter`.
  - Se fallisce per un tipo: quell'evento non arriva. Guarda `errors` nel file (voce 15), poi verifica il nome dell'evento nel
    client Forever (può differire da quello retail). Annota quale manca.

- [ ] **9. `/reload` a metà serata continua la stessa sessione.** Dopo `/reload` il file ha ancora una sola sessione per la serata
  (stesso `id`), senza `stop`, con i nuovi eventi accodati.
  - Se fallisce (due sessioni): `PLAYER_ENTERING_WORLD` non arriva con `isReload` vero. Annota gli argomenti reali e correggi
    `Main.lua`.

## API di gioco da verificare

- [ ] **10. Abbandono di una missione.** Abbandona una missione di prova (poi riprendila). Nel file deve esserci un solo evento
  `quest` con `action = "abandon"`, `id` e `title`. L'addon ricorda l'id quando il popup di conferma chiama
  `C_QuestLog.SetAbandonQuest`, e registra l'abbandono al post-hook di `C_QuestLog.AbandonQuest` o, se esiste, dell'`AbandonQuest`
  globale (mai due eventi entro 1 secondo). A mano: `/dump C_QuestLog.SetAbandonQuest, C_QuestLog.AbandonQuest, AbandonQuest`
  e `/dump C_QuestLog.GetAbandonQuest()` mentre il popup di conferma è aperto.
  - Se fallisce: se nessuna delle tre funzioni esiste nessun hook viene installato (nessun errore, solo nessun evento
    `abandon`). Se l'evento c'è ma senza `id`, `GetAbandonQuest()` restituisce 0 anche col popup aperto: annota cosa restituisce
    e da dove prende l'id il popup di Forever, oppure rinuncia agli abbandoni nella v1. Due eventi per un abbandono sono un bug.

- [ ] **11. `CHAT_MSG_LOOT` e `CHAT_MSG_SYSTEM` in dungeon (valori secret?).** In un dungeon ottieni un drop e un messaggio di
  sistema, poi controlla `errors` nel file e che il drop sia registrato.
  - Se fallisce (drop mancante in istanza): il messaggio arriva secret e viene scartato di proposito. Accetta la perdita oppure
    ricostruisci il drop da `LOOT_READY` + `GetLootSlotInfo`. Se invece compare un errore Lua è un bug: copia l'errore.

- [ ] **12. Nomi del gruppo e del boss in dungeon.** Entra in un dungeon con un gruppo: l'evento `group` deve avere i nomi
  (oppure mantenere l'ultimo gruppo noto se qualcuno è secret); l'evento `encounter` deve avere `name`.
  - Se fallisce: se `members` è sempre vuoto in istanza i nomi sono secret e il Piano 2 deve vivere con `group` assente. Se
    `encounter` non ha `name` va bene: restano `id`, `ok` e i tempi.

## Pulizia e stabilità

- [ ] **13. Crash simulato.** Gioca qualche minuto, scrivi `/reload` (le SavedVariables vengono scritte su disco), gioca ancora
  un po' e poi chiudi il client dal Task Manager. Riaccedi, scrivi `/reload` (serve a scrivere il file) e solo allora apri
  `DiarioGilda.lua`. Risultato atteso:
  - gli eventi fatti dopo il primo `/reload` mancano: dopo un kill il client non scrive le SavedVariables. È il comportamento
    del client: nelle serate lunghe fai un `/reload` ogni tanto;
  - la sessione della serata interrotta c'è, con gli eventi fino al primo `/reload` e `stop` = ora di quel `/reload` (lo scrive
    il logout dell'interfaccia; coincide più o meno con il suo ultimo evento). Non resta aperta;
  - c'è una nuova sessione per il login dopo il crash, senza `stop` (in corso), con un `id` diverso.
  - Se fallisce: se la sessione interrotta manca del tutto, il primo `/reload` non ha scritto il file (ripeti aspettando
    qualche secondo dopo il `/reload`). Se la sessione interrotta è senza `stop`, o la nuova sessione manca o ha lo stesso `id`,
    è un bug in `Journal.begin`: copia le sessioni (`id`, `start`, `stop`, `t` dell'ultimo evento).

- [ ] **14. Dimensione del file dopo una serata.** Dopo 2-3 ore il file `DiarioGilda.lua` deve restare sotto qualche centinaio
  di KB. Il tetto è 5000 eventi per sessione (`dropped` conta il resto).
  - Se fallisce (file enorme o `dropped` alto): guarda quale tipo `k` domina (di solito `zone` o `combat`) e riduci la
    frequenza di quell'evento in `Recorder.lua`.

- [ ] **15. Nessun errore Lua.** Con `/console scriptErrors 1` non deve apparire nessuna finestra di errore in tutta la serata, e
  `errors` nel file deve essere vuoto o spiegabile.
  - Se fallisce: copia il testo dell'errore (o la riga di `errors` nel file) con nome del file e riga. Ogni handler è protetto
    da `pcall`, quindi l'errore finisce in `errors` e non rompe il gioco. Correggi dal messaggio, non a occhio.

## Esito

Annota qui la data e per ogni voce V (ok) o X (fallita, con cosa hai trovato) prima di passare al Piano 2.
