"""Dopo `chiudi`, chiede a GitHub di far partire subito la Voce del Saggio (il video della pagina).

Il cron del workflow `saggio.yml` resta come riserva: GitHub non garantisce gli orari programmati
(il 7/11 non è partito nemmeno una volta tra le 07:00 e le 08:45). Un errore qui non ferma mai il
Cronista: la pagina e lo stato sono già salvati, al massimo il video arriva col cron.

Nel cloud il token manca: l'header Authorization per api.github.com lo aggiunge il proxy
dell'ambiente. In locale si può dare GITHUB_TOKEN_SAGGIO (solo permesso Actions su questo repo).
"""
from __future__ import annotations

import json
import os
import urllib.error
import urllib.request

from .discord import _APRI, USER_AGENT

BASE_PREDEFINITA = "https://api.github.com"
PERCORSO = "/repos/oLdpZ/diario-gilda/actions/workflows/saggio.yml/dispatches"


def avvia() -> bool:
    """Chiede l'avvio del workflow. Stampa com'è andata; True se GitHub l'ha accettato."""
    base = (os.environ.get("CRONISTA_GITHUB") or BASE_PREDEFINITA).rstrip("/")
    token = os.environ.get("GITHUB_TOKEN_SAGGIO")
    corpo = json.dumps({"ref": "main", "inputs": {"prova": "false"}}).encode()
    req = urllib.request.Request(base + PERCORSO, data=corpo, method="POST")
    req.add_header("User-Agent", USER_AGENT)
    req.add_header("Accept", "application/vnd.github+json")
    req.add_header("X-GitHub-Api-Version", "2022-11-28")
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    try:
        with _APRI(req, timeout=30) as r:
            codice = r.status
    except urllib.error.HTTPError as e:
        codice = e.code
        e.close()
    except (OSError, ValueError) as e:
        print(f"Avviso: Voce del Saggio non avviata ({type(e).__name__}); il video arriverà col giro programmato.")
        return False
    if codice == 204:
        print("Voce del Saggio avviata: il video arriverà tra qualche minuto.")
        return True
    print(f"Avviso: Voce del Saggio non avviata (GitHub HTTP {codice}); il video arriverà col giro programmato.")
    return False
