# Diario di Gilda: preparazione di Discord, GitHub e delle release (per l'autore)

Da fare una volta sola prima di distribuire il programma, poi a ogni nuova versione (ultima sezione).

## 1. Canali Discord

Nel server della gilda crea due canali di testo:

- **`#diario-di-gilda`**: qui il bot pubblica il racconto delle sessioni. È il canale che leggono tutti.
- **`#diario-servizio`**: qui il programma invia i file delle sessioni (uno per sessione, `sessione_….json.gz`).
  Lascialo **visibile** ai membri (serve anche per l'invio a mano descritto nella guida di installazione), ma
  imposta le notifiche del canale su **Niente** (tasto destro sul canale → Silenzia canale / Impostazioni di
  notifica) e suggerisci ai membri di fare lo stesso.

## 2. Webhook su `#diario-servizio`

1. Impostazioni di `#diario-servizio` → **Integrazioni** → **Webhook** → **Nuovo webhook**.
2. Nome: `Diario di Gilda`. Canale: `#diario-servizio`.
3. **Copia URL del webhook**. Chi lo ha può scrivere in `#diario-servizio`. Non metterlo mai nel codice, in un
   commit o in chat; va solo nel segreto `WEBHOOK_URL` di GitHub (sezione 5).
   Attenzione: il workflow lo incorpora nel programma, e il programma è scaricabile da chiunque dalle release
   pubbliche, quindi chi si impegna può estrarlo. Per questo il webhook punta solo a `#diario-servizio`.
   Se viene usato male (messaggi estranei nel canale): elimina il webhook in Discord, creane uno nuovo, metti il
   nuovo URL nel segreto `WEBHOOK_URL` e fai una nuova release (sezione 6). Le app si aggiornano da sole e usano
   il webhook nuovo; finché non si aggiornano, i loro invii al webhook vecchio falliscono e restano in coda.

## 3. Applicazione bot

1. Su <https://discord.com/developers/applications> → **New Application** (`Diario di Gilda`) → **Bot**.
   Prendi il token del bot e tienilo da parte per il Piano 3 (non va in questo repository).
2. **OAuth2 → URL Generator**: scope `bot`, nessun permesso a livello di server. Apri l'URL e aggiungi il bot
   al server della gilda.
3. Permessi **solo su quei due canali**: nelle impostazioni di `#diario-di-gilda` e di `#diario-servizio` →
   **Permessi** → aggiungi il bot (o il suo ruolo) e concedi:
   - `View Channel` (Visualizzare il canale)
   - `Read Message History` (Leggere la cronologia dei messaggi)
   - `Send Messages` (Inviare messaggi)
   - `Embed Links` (Incorporare link)
   - `Attach Files` (Allegare file)

   Su tutti gli altri canali il bot non deve avere permessi.

## 4. Repository pubblico `oLdpZ/diario-gilda`

1. Su GitHub crea il repository **pubblico** `oLdpZ/diario-gilda` (solo codice: nessun segreto, nessun dato di
   gioco). Deve essere pubblico perché il programma installato legge le release senza credenziali.
2. Carica il codice: `git remote add origin https://github.com/oLdpZ/diario-gilda.git` e `git push -u origin main`
   (dopo aver unito il ramo di lavoro in `main`).
3. In **Settings → Actions → General** lascia attive le Actions (il workflow di release chiede da solo il
   permesso di scrittura che gli serve).

## 5. Chiavi di firma, segreti e variabile

1. Sul tuo PC, dalla cartella del progetto: `go run ./app/cmd/firma keygen`. Stampa due righe:
   `PUBLIC_KEY=…` e `FIRMA_KEY=…`.
2. Su GitHub, **Settings → Secrets and variables → Actions**:
   - scheda **Secrets** → **New repository secret**:
     - `WEBHOOK_URL` = l'URL del webhook della sezione 2;
     - `FIRMA_KEY` = il valore dopo `FIRMA_KEY=` (chiave privata: firma gli aggiornamenti);
   - scheda **Variables** → **New repository variable**:
     - `PUBLIC_KEY` = il valore dopo `PUBLIC_KEY=` (pubblica: viene incorporata nel programma).
3. Conserva `FIRMA_KEY` anche in un posto sicuro fuori da GitHub (un gestore di password). Se la perdi, le app già
   installate non accetteranno più aggiornamenti firmati con una chiave nuova: i giocatori dovrebbero
   riscaricare il programma a mano. Non cancellare la finestra del terminale con le chiavi prima di averle salvate.

Per una build di prova sul tuo PC (solo Windows):

```bash
read -rs DIARIO_WEBHOOK && export DIARIO_WEBHOOK   # incolla l'URL del webhook e premi Invio: non si vede
DIARIO_PUBKEY='<PUBLIC_KEY>' bash scripts/build-app.sh
```

Non scrivere il webhook (né `FIRMA_KEY`) direttamente sulla riga di comando: finirebbe nella cronologia della shell.
`read -s` lo legge senza mostrarlo e senza salvarlo; se te ne dimentichi, lo script te lo chiede da solo nello
stesso modo. Crea `dist/DiarioGilda-windows-amd64.zip`; per avere anche il `.sig`, prima del comando leggi la
chiave allo stesso modo: `read -rs FIRMA_KEY && export FIRMA_KEY`.

## 6. Fare una release

La versione dell'addon e quella del programma sono la stessa. Sta scritta in due posti, che devono coincidere
(lo controllano i test Lua e il workflow):

1. Cambia la versione (per esempio da `0.1.0` a `0.1.1`) in:
   - `addon/DiarioGilda/DiarioGilda.toc`, riga `## Version: 0.1.1`;
   - `addon/DiarioGilda/Core/Const.lua`, riga `VERSION = "0.1.1",`.
2. Commit: `git commit -am "release 0.1.1"` e `git push`.
3. Tag e push del tag:

   ```bash
   git tag v0.1.1
   git push origin v0.1.1
   ```

4. Il workflow **Release** (scheda Actions) esegue i test Go e Lua, controlla che il tag sia uguale alla
   versione del TOC e di `Const.lua`, compila Windows e Mac, firma gli zip e pubblica la release con
   `DiarioGilda-windows-amd64.zip`, `DiarioGilda-macos.zip` e i due `.sig`.
5. Le app installate controllano gli aggiornamenti da sole, scaricano la nuova versione, verificano la firma e
   al riavvio reinstallano anche l'addon nuovo (se WoW è aperto, il giocatore vede "Addon aggiornato: scrivi /reload in gioco").

Se il workflow fallisce:

1. Se nella pagina **Releases** è comparsa una release `v0.1.1` (anche parziale o in bozza), cancellala
   (**Delete**), altrimenti il nuovo tentativo si ferma perché la release esiste già.
2. Se l'errore è nel codice o nelle versioni: correggi, fai commit e push, poi sposta il tag sul nuovo commit:

   ```bash
   git tag -d v0.1.1 && git push origin :refs/tags/v0.1.1
   git tag v0.1.1 && git push origin v0.1.1
   ```

   Se l'errore era passeggero (rete, runner), basta **Re-run all jobs** nella scheda Actions, dopo aver
   cancellato l'eventuale release parziale.

Un tag con un suffisso (per esempio `v0.1.2-test`, con `0.1.2-test` nel TOC e in `Const.lua`) viene pubblicato come
pre-release: le app installate lo ignorano, quindi serve per provare il workflow senza toccare i giocatori.
