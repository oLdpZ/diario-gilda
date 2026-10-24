# Il Cronista della Gilda: preparazione del bot (per l'autore)

Il Cronista è il bot che ogni mattina legge i pacchetti delle sessioni in `#diario-servizio` e pubblica
la pagina di diario in `#diario-di-gilda`. Gira come **attività programmata di Claude Code nel cloud**
(claude.ai/code/routines) con il tuo abbonamento: il PC può restare spento.

Il server è **Orda Florentiae** (id `1562010080940130304`). `bot/config.json` contiene solo l'id del
server, i nomi dei canali (`diario-di-gilda`, `diario-servizio`), il fuso orario e l'ora d'inizio della
giornata (06:00). Nessun segreto: il repository è pubblico.

Le sezioni 1–3 sono già fatte; restano le sezioni 4–7.

## 1. Applicazione e bot — fatto

1. Su <https://discord.com/developers/applications> → **New Application** → nome `Cronista della Gilda`.
2. Scheda **Bot**:
   - **Message Content Intent** attivo (Privileged Gateway Intents). Senza, l'API REST restituisce vuoti gli
     allegati dei messaggi del webhook e il Cronista non vede i pacchetti. Sotto i 100 server basta
     l'interruttore, nessuna verifica.
   - **Reset Token** → copia il token. Serve solo nella sezione 4 (e per la prova in locale, sezione 7).
     Non metterlo mai nel repository, in un file o in chat. Se finisce dove non deve: **Reset Token** di
     nuovo e aggiorna il Network secret.

## 2. Invito nel server — fatto

Link di invito (sostituisci `ID_APPLICAZIONE` con l'*Application ID* della scheda **General Information**):

```
https://discord.com/oauth2/authorize?client_id=ID_APPLICAZIONE&scope=bot&permissions=117760
```

`117760` = `View Channel` + `Send Messages` + `Embed Links` + `Attach Files` + `Read Message History`.

## 3. Permessi sui canali — fatto

In `#diario-di-gilda` e `#diario-servizio` → **Modifica canale → Permessi** → il bot (o il suo ruolo):
`View Channel`, `Read Message History`, `Send Messages`, `Embed Links`, `Attach Files`. Negli altri canali
il bot non serve.

## 4. Ambiente cloud con il token (Network secret)

Il token non va mai nell'ambiente né nei file: lo aggiunge il proxy del cloud a ogni richiesta verso
`discord.com`. Il Cronista, quando non trova `DISCORD_BOT_TOKEN`, non manda l'header e lascia fare al proxy.

1. Apri <https://claude.ai/code/routines> → impostazioni degli **ambienti** (Environments) → **Nuovo
   ambiente**, nome per esempio `cronista`.
2. **Accesso di rete:** *Custom*. Aggiungi i domini `discord.com`, `cdn.discordapp.com` e
   `media.discordapp.net` e lascia attivi i gestori di pacchetti predefiniti. Gli allegati (i pacchetti
   delle sessioni e `stato.json`) si scaricano da `cdn.discordapp.com` / `media.discordapp.net`: senza
   questi due domini il Cronista non legge nulla.
3. **Network secret:** nuovo segreto **solo** per il dominio `discord.com` (non per i due domini degli
   allegati: lì il token non deve andare):
   - header: `Authorization`
   - prefisso: `Bot` (il proxy manda `Authorization: Bot <token>`)
   - valore: il token del bot (sezione 1).
4. Nessuna variabile d'ambiente da aggiungere. Non serve installare nulla: il Cronista usa solo Python.
5. Salva.

## 5. Attività programmata

1. Sempre su <https://claude.ai/code/routines> → **Nuova attività** (New routine).
2. Repository: `oLdpZ/diario-gilda`, ramo `main` (il Cronista deve essere già unito in `main`).
3. Ambiente: quello della sezione 4.
4. Pianificazione: **ogni giorno alle 10:07** (ora italiana). I minuti non tondi partono più puntuali.
5. Prompt, una riga sola:

   ```
   Segui bot/ISTRUZIONI.md
   ```

6. Salva.

Ogni esecuzione clona il repository da zero, quindi parte sempre pulita. Il Cronista non scrive nulla nel
repository: pagine e riassunti stanno in `bot/lavoro/` (escluso da git) e spariscono a fine esecuzione; la
memoria (giornate pubblicate, numero del giorno, contatori, ricorrenze) è il messaggio `Stato del Cronista`
con l'allegato `stato.json` in `#diario-servizio`. Non cancellare quei messaggi: l'ultimo è la memoria.

## 6. Prima esecuzione a mano

1. Nell'attività appena creata premi **Esegui ora** (Run now).
2. Segui la sessione. Deve:
   - lanciare `python bot/cronista.py prepara`;
   - scrivere la pagina in `bot/lavoro/`;
   - lanciare `pubblica` e poi `chiudi`.
3. Controlla:
   - in `#diario-di-gilda` la pagina (un embed dorato firmato «Il Cronista della Gilda»);
   - in `#diario-servizio` un messaggio `Stato del Cronista — AAAA-MM-GG` con `stato.json`.
4. Casi normali:
   - *«Niente da raccontare oggi.»*: non c'erano pacchetti nuovi di giornate già finite. Non pubblica nulla.
   - **Primo avvio con più giornate**: una sola pagina di recupero, «Le Cronache dei primi giorni», poi una
     pagina al giorno.
   - La giornata in corso (dalle 06:00 di oggi) non viene mai raccontata: arriva domani.
   - Una sessione arrivata in ritardo (di una giornata già pubblicata) finisce nella pagina successiva.
5. Se qualcosa fallisce, l'esecuzione si ferma senza pubblicare pezzi di pagina e scrive l'errore nella
   risposta finale. Gli errori più comuni:
   - `Discord HTTP 401`: il Network secret manca o il token è sbagliato (sezione 4);
   - `Discord HTTP 403`: mancano permessi su un canale (sezione 3);
   - `canale non trovato` / `server non trovato`: nomi o id in `bot/config.json`;
   - errore di rete verso `discord.com`: l'accesso di rete *Custom* non include `discord.com`;
   - pacchetti «saltati» con un errore di rete (`Discord HTTP 0`) e stato mai trovato: mancano
     `cdn.discordapp.com` / `media.discordapp.net` nell'accesso di rete (sezione 4).

## 7. Prova in locale (facoltativa)

Dal tuo PC, nella cartella del progetto, con Python 3.11 o più recente. Il token va solo nella variabile
della finestra del terminale, mai in un file:

```
# Git Bash
read -rs DISCORD_BOT_TOKEN && export DISCORD_BOT_TOKEN   # incolla il token e premi Invio
python bot/cronista.py prepara
```

`prepara` legge soltanto: non pubblica e non cambia lo stato. Poi scrivi una pagina di prova in
`bot/lavoro/pagina-prova.md` (prima riga `# Titolo`) e mandala nel **canale di servizio**:

```
python bot/cronista.py pubblica bot/lavoro/pagina-prova.md --prova
```

`--prova` non conta come pubblicazione. Senza `--prova` la pagina andrebbe in `#diario-di-gilda` e lo
stato verrebbe salvato subito. **Non lanciare `chiudi`** nelle prove: segnerebbe le giornate come
pubblicate e l'attività del mattino non le racconterebbe più. A fine prova chiudi il terminale (la variabile
sparisce) e cancella `bot/lavoro/`.

Test automatici (non serve il token, usano un finto Discord):

```
python -m unittest discover -s bot/tests -t .
```
