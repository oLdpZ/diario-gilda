"""Stato del Cronista, conservato in #diario-servizio come allegato stato.json."""
from __future__ import annotations

import json
import time
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone

from .discord import Discord, DiscordError

PREFISSO = "Stato del Cronista"
NOME_FILE = "stato.json"
GIORNI_CONSERVAZIONE = 60
MAX_PAGINE = 50


@dataclass
class Stato:
    versione: int = 1
    ultimo_messaggio: str | None = None
    giorni_pubblicati: list[str] = field(default_factory=list)
    numero_giorno: int = 0
    # Forma scelta (R1): id sessione -> {"eventi": n, "visto": epoch secondi}
    sessioni_viste: dict[str, dict] = field(default_factory=dict)
    contatori: dict = field(default_factory=dict)
    ricorrenze: list[str] = field(default_factory=list)
    # personaggi principali ("nome|reame" in minuscolo) imparati dai pacchetti del programmino
    principali: list[str] = field(default_factory=list)

    def pota(self, ora: float | None = None) -> None:
        """Toglie le sessioni viste da più di 60 giorni."""
        limite = (time.time() if ora is None else ora) - GIORNI_CONSERVAZIONE * 86400
        def recente(v) -> bool:
            t = v.get("visto") if isinstance(v, dict) else None
            return isinstance(t, (int, float)) and not isinstance(t, bool) and t >= limite
        self.sessioni_viste = {k: v for k, v in self.sessioni_viste.items() if recente(v)}


def _intero(v) -> bool:
    return isinstance(v, int) and not isinstance(v, bool)


def _lista_str(v) -> bool:
    return isinstance(v, list) and all(isinstance(x, str) for x in v)


def _da_dict(d, ora: float) -> Stato:
    """Stato dal JSON; ValueError se la forma non è quella attesa."""
    if not isinstance(d, dict):
        raise ValueError("non è un oggetto JSON")
    s = Stato()
    um = d.get("ultimo_messaggio")
    if _intero(um):
        um = str(um)
    if um is not None and not (isinstance(um, str) and um.isdigit()):
        raise ValueError("ultimo_messaggio non valido")
    s.ultimo_messaggio = um
    for k, ok in (("giorni_pubblicati", _lista_str), ("ricorrenze", _lista_str), ("principali", _lista_str),
                  ("numero_giorno", lambda v: _intero(v) and v >= 0),
                  ("contatori", lambda v: isinstance(v, dict))):
        if k in d:
            if not ok(d[k]):
                raise ValueError(f"{k} non valido")
            setattr(s, k, d[k])
    viste = d.get("sessioni_viste", {})
    if not isinstance(viste, dict):
        raise ValueError("sessioni_viste non valido")
    for k, v in viste.items():
        # tollera la forma vecchia (solo numero di eventi)
        if isinstance(v, dict):
            s.sessioni_viste[k] = dict(v)
        elif _intero(v):
            s.sessioni_viste[k] = {"eventi": v, "visto": ora}
        else:
            raise ValueError(f"sessioni_viste[{k[:40]!r}] non valido")
    return s


def leggi(d: Discord, canale_id: str) -> Stato:
    """Cerca a ritroso l'ultimo `Stato del Cronista…` del bot con stato.json.

    Nessuno trovato a canale esaurito -> Stato() vuoto; dopo MAX_PAGINE pagine
    senza esito -> DiscordError (mai uno stato vuoto per errore).
    """
    io = d.io()
    prima = None
    for _ in range(MAX_PAGINE):
        pag = d.pagina_prima(canale_id, prima)
        for m in sorted(pag, key=lambda m: -int(m["id"])):
            if str(m.get("author", {}).get("id")) != io:
                continue
            if not m.get("content", "").startswith(PREFISSO):
                continue
            for a in m.get("attachments", []):
                if a.get("filename") == NOME_FILE:
                    dati = d.scarica(a["url"])
                    try:
                        return _da_dict(json.loads(dati.decode("utf-8")), time.time())
                    except (ValueError, RecursionError) as e:  # anche JSON e UTF-8 rotti
                        raise DiscordError(0, f"{NOME_FILE} rovinato nel messaggio {m['id']} di questo "
                                              f"canale ({str(e)[:120]}): va sistemato a mano") from None
        if len(pag) < 100:
            return Stato()
        prima = str(min(int(m["id"]) for m in pag))
    raise DiscordError(0, f"stato non trovato in {MAX_PAGINE} pagine di messaggi")


def scrivi(d: Discord, canale_id: str, s: Stato, ora: float | None = None) -> None:
    """Pota lo stato (in place) e lo pubblica come nuovo messaggio con allegato."""
    ora = time.time() if ora is None else ora
    s.pota(ora)
    data = datetime.fromtimestamp(ora, timezone.utc).strftime("%Y-%m-%d")
    dati = json.dumps(asdict(s), ensure_ascii=False, indent=1).encode("utf-8")
    d.invia(canale_id, f"{PREFISSO} — {data}", file=(NOME_FILE, dati))
