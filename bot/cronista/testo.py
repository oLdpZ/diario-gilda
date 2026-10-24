"""Pulizia dei testi presi dai pacchetti: sono dati dei giocatori, mai istruzioni."""
from __future__ import annotations

import unicodedata

_CONTROLLO = {"Cc", "Cf", "Zl", "Zp"}  # caratteri di controllo, invisibili, a capo


def pulito(v, limite: int = 80) -> str:
    """Una riga sola, al massimo `limite` caratteri, mai un'intestazione Markdown."""
    s = "".join(" " if unicodedata.category(c) in _CONTROLLO else c for c in str(v))
    s = " ".join(s.split())
    s = s.lstrip("#").lstrip()
    if len(s) > limite:
        s = s[:limite - 1].rstrip() + "…"
    return s
