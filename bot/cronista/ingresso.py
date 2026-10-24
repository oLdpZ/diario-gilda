"""Ingresso dei pacchetti di sessione dal canale di servizio, doppioni e giornate di gioco."""
from __future__ import annotations

import json
import zlib
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo

from . import luasv
from .discord import Discord, DiscordError
from .stato import Stato
from .testo import pulito

MAX_DECOMPRESSO = 32 * 1024 * 1024
MAX_EPOCH = 4_000_000_000
DODICI_ORE = 12 * 3600
EVENTO_PRIMA_S = 86400       # un evento puo' stare al piu' 1 giorno prima dello start
EVENTO_DOPO_S = 2 * 86400    # e al piu' 2 giorni dopo lo stop (o lo start)
NOME_LUA = "diariogilda.lua"
SUFFISSO_PACCHETTO = ".json.gz"


@dataclass
class Raccolta:
    sessioni: list[dict] = field(default_factory=list)
    avvisi: list[str] = field(default_factory=list)
    ultimo_messaggio: str | None = None
    # id sessione -> id del messaggio da cui viene la copia scelta
    messaggi: dict[str, str] = field(default_factory=dict)
    # personaggi principali ("nome|reame") imparati dai pacchetti di questo giro
    principali: set[str] = field(default_factory=set)


def conta_eventi(s: dict) -> int:
    """Eventi della sessione arricchita, compresi quelli tolti (`troncata`, `gia_visti`)."""
    extra = sum(n for n in (s.get("troncata"), s.get("gia_visti")) if _intero(n))
    return len(s["sessione"]["events"]) + extra


def fino(s: dict) -> int:
    """Istante dell'ultimo evento o episodio della sessione (anche di quelli già raccontati)."""
    prima = s.get("gia_fino")
    se = s["sessione"]
    da, a = se["start"] - EVENTO_PRIMA_S, (se.get("stop") or se["start"]) + EVENTO_DOPO_S
    episodi = [ep["to"] for ep in s["combattimenti"]
               if isinstance(ep, dict) and _intero(ep.get("to")) and da <= ep["to"] <= a]
    return max(_ultimo_t(s["sessione"]), prima if _intero(prima) else 0, *episodi)


def _solo_dopo(s: dict, t: int) -> dict:
    """Copia della sessione con i soli eventi ed episodi dopo `t` (il resto è già raccontato)."""
    ev = s["sessione"]["events"]
    nuovi = [e for e in ev if e["t"] > t]
    comb = [ep for ep in s["combattimenti"]
            if not (isinstance(ep, dict) and _intero(ep.get("to")) and ep["to"] <= t)]
    return dict(s, sessione=dict(s["sessione"], events=nuovi), combattimenti=comb,
                gia_visti=len(ev) - len(nuovi), gia_fino=t)


def _intero(v) -> bool:
    return isinstance(v, int) and not isinstance(v, bool)


def _controlla_sessione(se) -> tuple[dict, int]:
    """(sessione controllata, eventi scartati per orario `t` non valido)."""
    if not isinstance(se, dict):
        raise ValueError("sessione non valida")
    if not isinstance(se.get("id"), str) or not se["id"]:
        raise ValueError("sessione senza id")
    if not _intero(se.get("start")) or not 0 <= se["start"] <= MAX_EPOCH:
        raise ValueError("sessione senza start valido")
    ev = se.get("events", [])
    if ev == {}:
        ev = []
    if not isinstance(ev, list) or not all(isinstance(e, dict) for e in ev):
        raise ValueError("eventi non validi")
    if "stop" in se and (not _intero(se["stop"]) or not 0 <= se["stop"] <= MAX_EPOCH):
        raise ValueError("stop non valido")
    da, a = se["start"] - EVENTO_PRIMA_S, (se.get("stop") or se["start"]) + EVENTO_DOPO_S
    buoni = [e for e in ev if _intero(e.get("t")) and da <= e["t"] <= a]
    se["events"] = buoni
    return se, len(ev) - len(buoni)


def _scartati(n: int) -> list[str]:
    return [f"{n} eventi con orario non valido scartati"] if n else []


def _da_json_gz(dati: bytes) -> tuple[list[dict], list[str]]:
    try:
        d = zlib.decompressobj(31)  # 16+15: solo gzip
        grezzo = d.decompress(dati, MAX_DECOMPRESSO + 1)
        if len(grezzo) > MAX_DECOMPRESSO:
            raise ValueError("pacchetto troppo grande dopo la decompressione")
        if not d.eof:
            raise ValueError("gzip incompleto")
        doc = json.loads(grezzo.decode("utf-8"))
    except (zlib.error, UnicodeDecodeError, json.JSONDecodeError, EOFError,
            RecursionError, MemoryError) as e:
        raise ValueError(f"pacchetto illeggibile: {e}") from None
    if not isinstance(doc, dict):
        raise ValueError("documento non valido")
    if doc.get("formato") != 1:
        raise ValueError(f"formato non supportato: {doc.get('formato')!r}")
    if not isinstance(doc.get("giocatore"), str) or not doc["giocatore"]:
        raise ValueError("giocatore mancante")
    se, n = _controlla_sessione(doc.get("sessione"))
    comb = doc.get("combattimenti", [])
    if not isinstance(comb, list):
        raise ValueError("combattimenti non validi")
    tr = doc.get("troncata")
    out = {"giocatore": doc["giocatore"], "interrotta": bool(doc.get("interrotta", False)),
           "troncata": tr if _intero(tr) else 0, "sessione": se, "combattimenti": comb}
    if isinstance(doc.get("app"), str):
        out["app"] = doc["app"]
    return [out], _scartati(n)


def _ultimo_t(se: dict) -> int:
    ts = [e["t"] for e in se["events"] if _intero(e.get("t"))]
    return max(ts) if ts else se["start"]


def _da_lua(dati: bytes) -> tuple[list[dict], list[str]]:
    """Sessioni valide del .lua e un errore per ciascuna scartata."""
    try:
        db = luasv.carica(dati.decode("utf-8", "replace")).get("DiarioGildaDB")
    except (ValueError, RecursionError, MemoryError) as e:
        raise ValueError(f"file Lua illeggibile: {e}") from None
    if not isinstance(db, dict) or "sessions" not in db:
        raise ValueError("DiarioGildaDB.sessions non trovato")
    sess = db["sessions"]
    if isinstance(sess, dict) and not sess:
        sess = []
    if not isinstance(sess, list):
        raise ValueError("sessions non valido")
    buone, errori = [], []
    for i, se in enumerate(sess, 1):
        try:
            se, n = _controlla_sessione(se)
            nome = se.get("char")
            if not isinstance(nome, str) or not nome:
                raise ValueError("sessione senza char")
        except ValueError as e:
            errori.append(f"sessione {i} scartata: {e}")
            continue
        errori += [f"sessione {i}: {x}" for x in _scartati(n)]
        buone.append(se)
    # riferimento: l'istante piu' recente del file
    rif = max((max(_ultimo_t(se), se.get("stop", 0)) for se in buone), default=0)
    out = [{"giocatore": se["char"],
            "interrotta": "stop" not in se and _ultimo_t(se) < rif - DODICI_ORE,
            "troncata": 0, "sessione": se, "combattimenti": [], "manuale": True} for se in buone]
    return out, errori


VERSIONE_PRINCIPALE = (0, 1, 1)  # dal programmino 0.1.1 si invia solo il personaggio principale


def chiave_personaggio(se: dict) -> str:
    """Come si riconosce un personaggio: "nome|reame" in minuscolo."""
    return f"{str(se.get('char') or '').casefold()}|{str(se.get('realm') or '').casefold()}"


def _da_principale(s: dict) -> bool:
    """La sessione viene da un programmino che invia solo il personaggio principale."""
    try:
        v = tuple(int(x) for x in str(s.get("app", "")).split("-")[0].split("."))
    except ValueError:
        return False  # "dev" o assente
    return len(v) == 3 and v >= VERSIONE_PRINCIPALE


def _filtra_manuale(sessioni: list[dict], principali: set[str]) -> tuple[list[dict], list[str]]:
    """Le sessioni di un .lua caricato a mano che possono andare nel diario, e l'avviso sulle altre.

    Il .lua contiene tutti i personaggi dell'account, ma nel diario va solo il principale: si
    tengono i personaggi principali gia' noti; se nel file non ce n'e' nessuno, il personaggio con
    piu' eventi (a parita', il piu' recente).
    """
    chiavi = {chiave_personaggio(s["sessione"]) for s in sessioni}
    if len(chiavi) <= 1:
        return sessioni, []
    noti = chiavi & principali
    if noti:
        tenute = [s for s in sessioni if chiave_personaggio(s["sessione"]) in noti]
        perche = "si raccontano solo i personaggi principali"
    else:
        peso: dict[str, tuple[int, int]] = {}
        for s in sessioni:
            k, se = chiave_personaggio(s["sessione"]), s["sessione"]
            n, ultimo = peso.get(k, (0, 0))
            peso[k] = (n + len(se["events"]), max(ultimo, se["start"]))
        scelto = max(peso, key=lambda k: peso[k])
        tenute = [s for s in sessioni if chiave_personaggio(s["sessione"]) == scelto]
        perche = f"dal file si racconta solo il personaggio più giocato, {tenute[0]['sessione']['char']}"
    tenuti = {id(s) for s in tenute}
    altri = sorted({s["sessione"]["char"] for s in sessioni if id(s) not in tenuti})
    return tenute, [f"{len(sessioni) - len(tenute)} sessioni di altri personaggi ignorate "
                    f"({', '.join(altri)}): {perche}"]


def _leggi(nome: str, dati: bytes) -> tuple[list[dict], list[str]]:
    n = nome.lower()
    if n.endswith(SUFFISSO_PACCHETTO):
        return _da_json_gz(dati)
    if n == NOME_LUA:
        return _da_lua(dati)
    raise ValueError("tipo di file non riconosciuto")


def leggi_pacchetto(nome: str, dati: bytes) -> list[dict]:
    """Decodifica un allegato in sessioni arricchite; ValueError se illeggibile.

    Per un .lua le sessioni non valide sono scartate (se nessuna e' valida: ValueError).
    """
    sessioni, errori = _leggi(nome, dati)
    if errori and not sessioni:
        raise ValueError(errori[0])
    return sessioni


def _chiave_migliore(s: dict) -> tuple[int, int]:
    return conta_eventi(s), s["sessione"].get("stop") or 0


def raccogli(d: Discord, servizio_id: str, stato: Stato) -> Raccolta:
    """Scarica e decodifica i pacchetti nuovi; salta i corrotti con un avviso."""
    r = Raccolta(ultimo_messaggio=stato.ultimo_messaggio)
    io = d.io()
    migliori: dict[str, dict] = {}
    con_episodi: dict[str, dict] = {}  # miglior copia con combattimenti, per id
    nome_app: dict[str, str] = {}  # giocatore come lo scrive il programma (non il .lua)
    msg_episodi: dict[str, str] = {}
    blocco: int | None = None  # primo messaggio con errore di rete: si riprova al giro dopo
    manuali: list[tuple[str, str, list[dict]]] = []  # (messaggio, nome mostrato, sessioni) dei .lua

    def aggiungi(s: dict, mid: str) -> None:
        id_ = s["sessione"]["id"]
        if s["combattimenti"] and (id_ not in con_episodi or _chiave_migliore(s)
                                   > _chiave_migliore(con_episodi[id_])):
            con_episodi[id_] = s
            msg_episodi[id_] = mid
        if id_ not in migliori or _chiave_migliore(s) > _chiave_migliore(migliori[id_]):
            migliori[id_] = s
            r.messaggi[id_] = mid

    for m in d.messaggi_dopo(servizio_id, stato.ultimo_messaggio):
        mid = str(m["id"])
        if r.ultimo_messaggio is None or int(mid) > int(r.ultimo_messaggio):
            r.ultimo_messaggio = mid
        if str(m.get("author", {}).get("id")) == io:
            continue
        for a in m.get("attachments", []):
            nome = str(a.get("filename", ""))
            if not (nome.lower().endswith(SUFFISSO_PACCHETTO) or nome.lower() == NOME_LUA):
                continue
            mostra = pulito(nome, 60)  # nome e messaggi vengono dai dati: corti e su una riga
            try:
                buoni, scartate = _leggi(nome, d.scarica(a["url"]))
                for e in scartate:
                    r.avvisi.append(f"Pacchetto {mostra} (messaggio {mid}): {pulito(e, 120)}")
                if any(s.get("manuale") for s in buoni):
                    manuali.append((mid, mostra, buoni))  # filtrati a fine giro, coi principali noti
                    continue
                for s in buoni:
                    nome_app.setdefault(s["sessione"]["id"], s["giocatore"])
                    if _da_principale(s):
                        r.principali.add(chiave_personaggio(s["sessione"]))
                    aggiungi(s, mid)
            except DiscordError as e:
                r.avvisi.append(f"Pacchetto {mostra} (messaggio {mid}) saltato: {pulito(e, 120)}")
                if (e.status == 0 or e.status == 429 or e.status >= 500) and blocco is None:
                    blocco = int(mid)
            except (ValueError, KeyError) as e:
                r.avvisi.append(f"Pacchetto {mostra} (messaggio {mid}) saltato: {pulito(e, 120)}")
    principali = set(stato.principali) | r.principali
    for mid, mostra, buoni in manuali:
        tenute, ignorate = _filtra_manuale(buoni, principali)
        r.avvisi += [f"Pacchetto {mostra} (messaggio {mid}): {pulito(e, 200)}" for e in ignorate]
        for s in tenute:
            aggiungi(s, mid)
    for id_, s in list(migliori.items()):
        manuale = bool(s.get("manuale"))
        altra = con_episodi.get(id_)
        if not s["combattimenti"] and altra is not None:
            # copie unite: eventi dalla vincitrice, episodi (e app) dalla copia che li ha
            s = dict(s, combattimenti=altra["combattimenti"])
            s.pop("manuale", None)
            if "app" in altra:
                s["app"] = altra["app"]
            migliori[id_] = s
            # per rileggere entrambe le copie, il messaggio piu' vecchio delle due
            r.messaggi[id_] = str(min(int(r.messaggi[id_]), int(msg_episodi[id_])))
        if manuale and id_ in nome_app:
            s = migliori[id_] = dict(s, giocatore=nome_app[id_])
        visto = stato.sessioni_viste.get(id_)
        if not isinstance(visto, dict):
            visto = None
        gia = visto.get("eventi") if visto else None
        if not _intero(gia) or conta_eventi(s) > gia:
            t = visto.get("fino") if visto else None
            r.sessioni.append(_solo_dopo(s, t) if _intero(gia) and _intero(t) else s)
        else:
            r.messaggi.pop(id_, None)
    if blocco is not None and r.ultimo_messaggio is not None and int(r.ultimo_messaggio) >= blocco:
        r.ultimo_messaggio = str(blocco - 1)
    r.sessioni.sort(key=lambda s: (s["sessione"]["start"], s["sessione"]["id"]))
    return r


def giornata(start_epoch: int, fuso: str, inizio: str) -> str:
    """Data (AAAA-MM-GG) della giornata di gioco: inizia alle `inizio` locali."""
    h, m = (int(x) for x in inizio.split(":"))
    local = datetime.fromtimestamp(start_epoch, ZoneInfo(fuso))
    if (local.hour, local.minute) < (h, m):
        local -= timedelta(days=1)
    return local.strftime("%Y-%m-%d")


def per_giornata(sessioni: list[dict], fuso: str = "Europe/Rome",
                 inizio: str = "06:00") -> dict[str, list[dict]]:
    """Raggruppa le sessioni per giornata del loro `start`, giorni in ordine."""
    out: dict[str, list[dict]] = {}
    for s in sessioni:
        out.setdefault(giornata(s["sessione"]["start"], fuso, inizio), []).append(s)
    return dict(sorted(out.items()))
