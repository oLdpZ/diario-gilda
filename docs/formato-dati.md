# Formato dei dati (`DiarioGildaDB`, versione 1)

Questo è il contratto con il Piano 2. Ogni cambiamento successivo aggiorna questo file.

```lua
DiarioGildaDB = {
  v = 1,
  sessions = {            -- array, in ordine di inizio
    { id = "Realm-Char-1793000000", char = "Gino", realm = "Realm", guild = "Nome gilda" --[[o nil]],
      class = "WARRIOR", race = "Human", level = 12, build = "1.60.1.70338", addon = "0.1.0",
      start = 1793000000, stop = 1793007200 --[[nil se in corso o crash]], dropped = 0,
      events = { { t = 1793000123, k = "zone", z = "Westfall", s = "Longshore", m = 1436, x = 0.512, y = 0.301, ... } },
    },
  },
  seen = { ["Realm-Char"] = { ["Westfall"] = true } },   -- zone già visitate (per `first`)
  errors = { "…" },                                        -- ultimi 20 errori dell'addon
}
```

## Campi assenti

Ogni campo di una sessione o di un evento può mancare se il client non lo ha reso leggibile (valore secret in dungeon,
API che restituisce nil o va in errore, dato non ancora caricato): l'evento viene registrato lo stesso, senza quel campo.
Esempi: `name` di `encounter`, `title` di `quest`, `src` di `loot`, `x`/`y`, `guild` della sessione (letta anche più tardi,
con `PLAYER_GUILD_UPDATE`), `diff` e `size` di `encounter`, `minHp` di `combat` (assente se durante il combattimento non c'è
stata nessuna lettura valida della vita), `id` di `quest`. Chi legge il file controlla sempre la presenza del campo.

## Ciclo di vita delle sessioni

- **In corso:** `stop` assente. Può essere anche una sessione interrotta (vedi sotto).
- **Riaperta da `/reload`:** un `/reload` scatena il logout dell'interfaccia, che scrive `stop`. Se entro `RELOAD_GAP`
  (600 s) dopo `stop` lo stesso personaggio rientra con un `/reload`, la stessa sessione viene riaperta: stesso `id`, `stop`
  di nuovo assente, nuovi eventi accodati. Quindi una sessione con `stop` può ancora cambiare.
- **Definitiva:** `stop` presente **e** (`time() - stop > 600` **oppure** esiste una sessione più recente dello stesso
  `realm-char`). Solo le sessioni definitive vanno considerate chiuse per sempre.
- **Interrotta (crash):** `stop` assente e l'ultimo evento (o `start`, se non ci sono eventi) è più vecchio di 12 ore. Va
  trattata come chiusa all'ultimo evento. Al login successivo dello stesso personaggio l'addon la chiude da sé con
  `stop = t dell'ultimo evento` (o `start`).
- Le sessioni con `start` più vecchio di 14 giorni vengono potate al login.

## Eventi

Campi comuni a ogni evento: `t` (epoch), `k` (tipo), `z` zona, `s` sottozona, `m` uiMapID, `x`/`y` posizione 0–1 a 3 decimali (assenti in istanza), `inst` nome istanza se in istanza.

| `k` | campi propri | quando |
|---|---|---|
| `zone` | `first` (bool) | cambio zona (`ZONE_CHANGED_NEW_AREA`) e al login |
| `loot` | `item`, `id`, `q`, `n`, `src` (`"fishing"`, nome del mob, o assente: la fonte vale 10 s e solo finché la finestra di bottino è aperta) | oggetto ricevuto di qualità `q >= 3` |
| `craft` | `item`, `id`, `q`, `n` | oggetto creato di qualità `q >= 3` |
| `fishing` | `from`, `to`, `casts`, `catches` (array di nomi, max 20) | chiuso dopo 120 s senza lanci né catture, al logout o all'inizio di un combattimento |
| `level` | `level` | level up |
| `death` | `breath` (bool: timer del respiro attivo) | morte del giocatore |
| `combat` | `from`, `to`, `minHp` (0–100, vita minima letta, compresa quella all'ingresso; assente senza letture) | fine combattimento del giocatore |
| `encounter` | `id`, `name`, `diff`, `size`, `ok` (bool), `from`, `to` | fine incontro con un boss |
| `group` | `members` (nomi), `guild` (nomi in gilda) | composizione del gruppo cambiata e stabile da 5 s; gruppo vuoto = `members = {}` |
| `quest` | `id`, `title`, `action` (`"done"`/`"abandon"`), `xp`, `money` | missione consegnata o abbandonata |
| `duel` | `winner`, `loser`, `how` (`"knockout"`/`"retreat"`) | messaggio di sistema di fine duello |
| `spend` | `amount` (rame), `where` (`"merchant"`/`"auction"`/`"trainer"`/`"other"`) | calo dei soldi `>= 50000` rame (5 oro) in un colpo |
| `note` | `text` | solo messaggi dell'addon stesso (es. registro combattimenti spento) |
