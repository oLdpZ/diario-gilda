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

Campi comuni a ogni evento: `t` (epoch), `k` (tipo), `z` zona, `s` sottozona, `m` uiMapID, `x`/`y` posizione 0–1 a 3 decimali (assenti in istanza), `inst` nome istanza se in istanza.

| `k` | campi propri | quando |
|---|---|---|
| `zone` | `first` (bool) | cambio zona (`ZONE_CHANGED_NEW_AREA`) e al login |
| `loot` | `item`, `id`, `q`, `n`, `src` (`"fishing"`, nome del mob, o assente) | oggetto ricevuto di qualità `q >= 3` |
| `craft` | `item`, `id`, `q`, `n` | oggetto creato di qualità `q >= 3` |
| `fishing` | `from`, `to`, `casts`, `catches` (array di nomi, max 20) | chiuso dopo 120 s senza lanci, al logout o all'inizio di un combattimento |
| `level` | `level` | level up |
| `death` | `breath` (bool: timer del respiro attivo) | morte del giocatore |
| `combat` | `from`, `to`, `minHp` (0–100) | fine combattimento del giocatore |
| `encounter` | `id`, `name`, `diff`, `size`, `ok` (bool), `from`, `to` | fine incontro con un boss |
| `group` | `members` (nomi), `guild` (nomi in gilda) | composizione del gruppo cambiata e stabile da 5 s; gruppo vuoto = `members = {}` |
| `quest` | `id`, `title`, `action` (`"done"`/`"abandon"`), `xp`, `money` | missione consegnata o abbandonata |
| `duel` | `winner`, `loser`, `how` (`"knockout"`/`"retreat"`) | messaggio di sistema di fine duello |
| `spend` | `amount` (rame), `where` (`"merchant"`/`"auction"`/`"trainer"`/`"other"`) | calo dei soldi `>= 50000` rame (5 oro) in un colpo |
| `note` | `text` | solo messaggi dell'addon stesso (es. registro combattimenti spento) |
