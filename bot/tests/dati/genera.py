"""Rigenera i pacchetti d'esempio: python -m bot.tests.dati.genera (dalla radice).

Formato: docs/formato-dati.md ("File inviato a Discord"). gzip con mtime 0 (riproducibile).
"""
from __future__ import annotations

import gzip
import json
from pathlib import Path

QUI = Path(__file__).parent


def _ep(kind, da, a, **k):
    base = {"from": da, "to": a, "kind": kind, "enemyCount": 0, "enemies": [], "damageTaken": 0,
            "healers": [], "environmental": [], "kills": []}
    base.update(k)
    return base


def gino() -> dict:
    t0 = 1793000000  # 2026-10-26 08:33 Europe/Rome
    ev = [
        {"t": t0, "k": "zone", "z": "Westfall", "s": "Longshore", "m": 1436, "x": 0.512, "y": 0.301, "first": True},
        {"t": t0 + 60, "k": "fishing", "z": "Westfall", "s": "Longshore", "m": 1436, "x": 0.512, "y": 0.301,
         "from": t0 + 15, "to": t0 + 50, "casts": 3, "catches": ["Pesce Gatto"]},
        {"t": t0 + 80, "k": "combat", "z": "Westfall", "s": "Longshore", "m": 1436, "x": 0.512, "y": 0.301,
         "from": t0 + 62, "to": t0 + 80, "minHp": 12},
        {"t": t0 + 110, "k": "loot", "z": "Westfall", "s": "Longshore", "m": 1436, "item": "Spada Rara",
         "id": 1234, "q": 3, "n": 1, "src": "Murloc Costiero"},
    ]
    comb = [_ep("combat", t0 + 62, t0 + 80, minHp=12, enemyCount=6,
                enemies=[{"name": "Murloc Costiero", "n": 6}], damageTaken=310,
                biggestHitTaken={"who": "Murloc Costiero", "spell": "Mischia", "amount": 41, "crit": False},
                healers=[{"name": "Gino", "amount": 120}],
                kills=[{"name": "Murloc Costiero", "n": 2}])]
    return {"formato": 1, "giocatore": "Gino", "app": "0.1.0", "interrotta": False,
            "sessione": {"id": "Realm-Gino-1793000000", "char": "Gino", "realm": "Realm",
                         "guild": "I Murloc Rampanti", "class": "WARRIOR", "race": "Human", "level": 12,
                         "build": "1.60.1.70338", "addon": "0.1.0", "start": t0, "stop": t0 + 1395,
                         "dropped": 0, "events": ev},
            "combattimenti": comb}


def mara() -> dict:
    t0 = 1793010000
    base = {"z": "Deadmines", "inst": "Deadmines", "m": 291}
    ev = [{"t": t0, "k": "zone", **base, "first": True}]
    ev.append({"t": t0, "k": "group", **base, "members": ["Mara", "Gino"], "guild": ["Mara", "Gino"]})
    comb = []
    for i in range(3):
        t = t0 + 300 + i * 200
        ev.append({"t": t, "k": "encounter", **base, "id": 593, "name": "Edwin VanCleef", "diff": 1,
                   "size": 5, "ok": False, "from": t - 90, "to": t})
        ev.append({"t": t + 5, "k": "death", **base, "breath": False})
        comb.append(_ep("death", t - 10, t + 5, enemyCount=2, enemies=[{"name": "Edwin VanCleef", "n": 1}, {"name": "Difensore Defias", "n": 1}],
                        damageTaken=900, killedBy={"who": "Edwin VanCleef", "spell": "Mischia", "amount": 210, "crit": True}))
    t = t0 + 1100
    ev.append({"t": t, "k": "encounter", **base, "id": 593, "name": "Edwin VanCleef", "diff": 1,
               "size": 5, "ok": True, "from": t - 120, "to": t})
    ev.append({"t": t + 20, "k": "loot", **base, "item": "Lama dei Defias", "id": 5187, "q": 4, "n": 1,
               "src": "Edwin VanCleef"})
    return {"formato": 1, "giocatore": "Mara", "app": "0.1.0", "interrotta": False,
            "sessione": {"id": "Realm-Mara-1793010000", "char": "Mara", "realm": "Realm",
                         "guild": "I Murloc Rampanti", "class": "MAGE", "race": "Gnome", "level": 18,
                         "build": "1.60.1.70338", "addon": "0.1.0", "start": t0, "stop": t0 + 1500,
                         "dropped": 0, "events": ev},
            "combattimenti": comb}


def gino_deadmines() -> dict:
    """Seconda sessione di Gino: la stessa serata a Deadmines con Mara (episodio condiviso)."""
    doc = mara()
    se = doc["sessione"]
    ev = [dict(e) for e in se["events"] if e["k"] != "loot"]
    ev[0]["t"] = 1793009990
    ev[1]["t"] = 1793009980  # il gruppo con Mara c'era gia' prima di entrare
    ev[0], ev[1] = ev[1], ev[0]
    return {"formato": 1, "giocatore": "Gino", "app": "0.1.0", "interrotta": False,
            "sessione": {"id": "Realm-Gino-1793009900", "char": "Gino", "realm": "Realm",
                         "guild": "I Murloc Rampanti", "class": "WARRIOR", "race": "Human", "level": 13,
                         "build": "1.60.1.70338", "addon": "0.1.0", "start": 1793009900,
                         "stop": 1793011600, "dropped": 0, "events": ev},
            "combattimenti": doc["combattimenti"]}


def _gz(doc) -> bytes:
    return gzip.compress(json.dumps(doc, ensure_ascii=False).encode("utf-8"), mtime=0)


def main() -> None:
    (QUI / "sessione_Gino_Realm-Gino-1793000000.json.gz").write_bytes(_gz(gino()))
    (QUI / "sessione_Mara_Realm-Mara-1793010000.json.gz").write_bytes(_gz(mara()))
    (QUI / "sessione_Gino_Realm-Gino-1793009900.json.gz").write_bytes(_gz(gino_deadmines()))
    (QUI / "sessione_corrotta.json.gz").write_bytes(
        gzip.compress(b'{"formato": 1, "giocatore": "Zed", "sessione": {"id": ', mtime=0)[:-9])


if __name__ == "__main__":
    main()
