"""Dopo `chiudi`, chiede a GitHub di far partire subito la Voce del Saggio (il video della pagina).

Il cron del workflow `saggio.yml` resta come riserva: GitHub non garantisce gli orari programmati
(il 7/11 non è partito nemmeno una volta tra le 07:00 e le 08:45). Un errore qui non ferma mai il
Cronista: la pagina e lo stato sono già salvati, al massimo il video arriva col cron.

Due modi, in quest'ordine:
1. workflow_dispatch (serve il permesso actions=write): funziona in locale con GITHUB_TOKEN_SAGGIO.
2. repository_dispatch, evento «cronaca» (serve contents=write). Nel cloud il token manca e
   l'header Authorization per api.github.com lo aggiunge il proxy: è quello dell'app GitHub di
   Claude, che il 9/11 ha rifiutato il primo modo («Resource not accessible by integration»,
   actions=write) ma può scrivere sul repo.
"""
from __future__ import annotations

import json
import os
import urllib.error
import urllib.request

from .discord import _APRI, USER_AGENT

BASE_PREDEFINITA = "https://api.github.com"
PERCORSO = "/repos/oLdpZ/diario-gilda/actions/workflows/saggio.yml/dispatches"
PERCORSO_REPO = "/repos/oLdpZ/diario-gilda/dispatches"
EVENTO = "cronaca"  # deve combaciare con `repository_dispatch: types` in saggio.yml


def _chiama(url: str, dati: dict, token: str | None) -> tuple[int | None, str]:
    """(codice HTTP o None se la rete ha fallito, dettagli dell'errore)."""
    req = urllib.request.Request(url, data=json.dumps(dati).encode(), method="POST")
    req.add_header("User-Agent", USER_AGENT)
    req.add_header("Accept", "application/vnd.github+json")
    req.add_header("X-GitHub-Api-Version", "2022-11-28")
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    try:
        with _APRI(req, timeout=30) as r:
            return r.status, ""
    except urllib.error.HTTPError as e:
        dettagli = _dettagli(e)
        e.close()
        return e.code, dettagli
    except (OSError, ValueError) as e:
        return None, type(e).__name__


def avvia() -> bool:
    """Chiede l'avvio del workflow. Stampa com'è andata; True se GitHub l'ha accettato."""
    base = (os.environ.get("CRONISTA_GITHUB") or BASE_PREDEFINITA).rstrip("/")
    token = os.environ.get("GITHUB_TOKEN_SAGGIO")
    c1, d1 = _chiama(base + PERCORSO, {"ref": "main", "inputs": {"prova": "false"}}, token)
    if c1 == 204:
        print("Voce del Saggio avviata: il video arriverà tra qualche minuto.")
        return True
    c2, d2 = _chiama(base + PERCORSO_REPO, {"event_type": EVENTO}, token)
    if c2 == 204:
        print("Voce del Saggio avviata (repository_dispatch): il video arriverà tra qualche minuto.")
        return True

    def esito(c, d):
        return (f"HTTP {c}" if c is not None else d) + (f" ({d})" if c is not None and d else "")
    print("Avviso: Voce del Saggio non avviata; il video arriverà col giro programmato. "
          f"workflow_dispatch: {esito(c1, d1)}. repository_dispatch: {esito(c2, d2)}.")
    return False


def _dettagli(e: urllib.error.HTTPError) -> str:
    """Messaggio d'errore, permesso richiesto e server: dicono se ha rifiutato GitHub o il proxy."""
    parti = []
    try:
        testo = e.read(2000).decode("utf-8", "replace")
    except OSError:
        testo = ""
    try:
        msg = json.loads(testo).get("message", "")
    except (ValueError, AttributeError):
        msg = " ".join(testo.split())[:200]
    if msg:
        parti.append(f"messaggio «{msg}»")
    h = e.headers
    if h is not None:
        if h.get("X-Accepted-GitHub-Permissions"):
            parti.append(f"permesso richiesto {h['X-Accepted-GitHub-Permissions']}")
        if h.get("Server"):
            parti.append(f"server {h['Server']}")
    return ", ".join(parti)
