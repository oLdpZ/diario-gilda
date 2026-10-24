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

MAX_DECOMPRESSO = 32 * 1024 * 1024
MAX_EPOCH = 4_000_000_000
DODICI_ORE = 12 * 3600
NOME_LUA = "diariogilda.lua"
SUFFISSO_PACCHETTO = ".json.gz"


@dataclass
class Raccolta:
    sessioni: list[dict] = field(default_factory=list)
    avvisi: list[str] = field(default_factory=list)
    ultimo_messaggio: str | None = None
    # id sessione -> id del messaggio da cui viene la copia scelta
    messaggi: dict[str, str] = field(default_factory=dict)


def conta_eventi(s: dict) -> int:
    """Eventi della sessione arricchita, compresi quelli tolti (`troncata`)."""
    n = s.get("troncata")
    return len(s["sessione"]["events"]) + (n if isinstance(n, int) and not isinstance(n, bool) else 0)


def _intero(v) -> bool:
    return isinstance(v, int) and not isinstance(v, bool)


def _controlla_sessione(se) -> dict:
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
    se["events"] = ev
    if "stop" in se and (not _intero(se["stop"]) or not 0 <= se["stop"] <= MAX_EPOCH):
        raise ValueError("stop non valido")
    return se


def _da_json_gz(dati: bytes) -> list[dict]:
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
    se = _controlla_sessione(doc.get("sessione"))
    comb = doc.get("combattimenti", [])
    if not isinstance(comb, list):
        raise ValueError("combattimenti non validi")
    tr = doc.get("troncata")
    out = {"giocatore": doc["giocatore"], "interrotta": bool(doc.get("interrotta", False)),
           "troncata": tr if _intero(tr) else 0, "sessione": se, "combattimenti": comb}
    if isinstance(doc.get("app"), str):
        out["app"] = doc["app"]
    return [out]


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
            se = _controlla_sessione(se)
            nome = se.get("char")
            if not isinstance(nome, str) or not nome:
                raise ValueError("sessione senza char")
        except ValueError as e:
            errori.append(f"sessione {i} scartata: {e}")
            continue
        buone.append(se)
    # riferimento: l'istante piu' recente del file
    rif = max((max(_ultimo_t(se), se.get("stop", 0)) for se in buone), default=0)
    out = [{"giocatore": se["char"],
            "interrotta": "stop" not in se and _ultimo_t(se) < rif - DODICI_ORE,
            "troncata": 0, "sessione": se, "combattimenti": [], "manuale": True} for se in buone]
    return out, errori


def _leggi(nome: str, dati: bytes) -> tuple[list[dict], list[str]]:
    n = nome.lower()
    if n.endswith(SUFFISSO_PACCHETTO):
        return _da_json_gz(dati), []
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
    blocco: int | None = None  # primo messaggio con errore di rete: si riprova al giro dopo
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
            try:
                buoni, scartate = _leggi(nome, d.scarica(a["url"]))
                for e in scartate:
                    r.avvisi.append(f"Pacchetto {nome} (messaggio {mid}): {e}")
                for s in buoni:
                    id_ = s["sessione"]["id"]
                    if id_ not in migliori or _chiave_migliore(s) > _chiave_migliore(migliori[id_]):
                        migliori[id_] = s
                        r.messaggi[id_] = mid
            except DiscordError as e:
                r.avvisi.append(f"Pacchetto {nome} (messaggio {mid}) saltato: {e}")
                if (e.status == 0 or e.status == 429 or e.status >= 500) and blocco is None:
                    blocco = int(mid)
            except (ValueError, KeyError) as e:
                r.avvisi.append(f"Pacchetto {nome} (messaggio {mid}) saltato: {e}")
    for id_, s in migliori.items():
        visto = stato.sessioni_viste.get(id_)
        if not isinstance(visto, dict):
            visto = None
        gia = visto.get("eventi") if visto else None
        if not _intero(gia) or conta_eventi(s) > gia:
            r.sessioni.append(s)
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
