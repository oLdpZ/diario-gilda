"""Stato del Cronista, conservato in #diario-servizio come allegato stato.json."""
from __future__ import annotations

import json
import time
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone

from .discord import Discord

PREFISSO = "Stato del Cronista"
NOME_FILE = "stato.json"
GIORNI_CONSERVAZIONE = 60


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

    def pota(self, ora: float | None = None) -> None:
        """Toglie le sessioni viste da più di 60 giorni."""
        limite = (time.time() if ora is None else ora) - GIORNI_CONSERVAZIONE * 86400
        self.sessioni_viste = {k: v for k, v in self.sessioni_viste.items()
                               if v.get("visto", 0) >= limite}


def _da_dict(d: dict, ora: float) -> Stato:
    s = Stato()
    for k in ("ultimo_messaggio", "giorni_pubblicati", "numero_giorno", "contatori", "ricorrenze"):
        if k in d:
            setattr(s, k, d[k])
    for k, v in d.get("sessioni_viste", {}).items():
        # tollera la forma vecchia (solo numero di eventi)
        s.sessioni_viste[k] = dict(v) if isinstance(v, dict) else {"eventi": int(v), "visto": ora}
    return s


def leggi(d: Discord, canale_id: str) -> Stato:
    """Legge l'ultimo messaggio `Stato del Cronista…` con stato.json; nessuno -> vuoto."""
    for m in reversed(d.ultimi(canale_id, 100)):
        if not m.get("content", "").startswith(PREFISSO):
            continue
        for a in m.get("attachments", []):
            if a.get("filename") == NOME_FILE:
                return _da_dict(json.loads(d.scarica(a["url"]).decode("utf-8")), time.time())
    return Stato()


def scrivi(d: Discord, canale_id: str, s: Stato, ora: float | None = None) -> None:
    """Pota lo stato (in place) e lo pubblica come nuovo messaggio con allegato."""
    ora = time.time() if ora is None else ora
    s.pota(ora)
    data = datetime.fromtimestamp(ora, timezone.utc).strftime("%Y-%m-%d")
    dati = json.dumps(asdict(s), ensure_ascii=False, indent=1).encode("utf-8")
    d.invia(canale_id, f"{PREFISSO} — {data}", file=(NOME_FILE, dati))
