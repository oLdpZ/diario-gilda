"""Dopo la pubblicazione su Discord, chiede al Cancelliere di mandare il video anche nel gruppo WhatsApp.

Il Cancelliere (Worker Cloudflare) lo mette nella coda del ponte e il bot WhatsApp lo invia alla fine
del silenzio notturno (08:00). Un errore qui non ferma mai il Saggio: il video su Discord c'è già.
"""
from __future__ import annotations

import json
import os
import urllib.error
import urllib.request

from ..cronista.discord import _APRI, USER_AGENT

URL_PREDEFINITO = "https://orda-bottega.oldpz1.workers.dev"


def annuncia(ref: str, titolo: str, messaggio: dict) -> bool:
    """Mette in coda il video del `messaggio` Discord appena inviato. True se il Cancelliere l'ha accettato."""
    segreto = (os.environ.get("WA_CRONACA_SECRET") or "").strip()
    if not segreto:
        print("  WhatsApp: WA_CRONACA_SECRET mancante, salto")
        return False
    allegati = messaggio.get("attachments") or []
    if not allegati or not allegati[0].get("url"):
        print("  WhatsApp NON avvisato: il messaggio non ha l'allegato")
        return False
    base = (os.environ.get("CANCELLIERE_URL") or URL_PREDEFINITO).rstrip("/")
    dati = {"ref": str(ref), "titolo": titolo, "video_url": allegati[0]["url"]}
    req = urllib.request.Request(base + "/wa/cronaca", data=json.dumps(dati).encode(), method="POST")
    req.add_header("User-Agent", USER_AGENT)
    req.add_header("Content-Type", "application/json")
    req.add_header("Authorization", f"Bearer {segreto}")
    try:
        with _APRI(req, timeout=30) as r:
            risposta = json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        e.close()
        print(f"  WhatsApp NON avvisato: HTTP {e.code}")
        return False
    except (OSError, ValueError) as e:
        print(f"  WhatsApp NON avvisato: {type(e).__name__}")
        return False
    print("  WhatsApp: in coda" if risposta.get("queued") else "  WhatsApp: già in coda")
    return True
