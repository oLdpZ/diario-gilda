"""Momenti candidati di una giornata di gioco, contatori tra i giorni e riassunto per Claude."""
from __future__ import annotations

import copy
import json
import math
from datetime import datetime
from zoneinfo import ZoneInfo

from .testo import pulito

UNIONE_S = 300          # episodi condivisi: entro 5 minuti
DOPO_MORTE_S = 30       # nessuna morte entro 30 s dalla fine del combattimento
PESCA_S = 120           # pesca finita entro 120 s prima della fuga
DROP_BOSS_S = 120       # bottino senza fonte entro 120 s da un boss ucciso
ORO_FOLLE = 100 * 10000  # 100 oro in rame
MAX_RIASSUNTO = 40
SALTA = (TypeError, ValueError, KeyError, AttributeError, IndexError, OverflowError)

NOMI_TIPO = {
    "drop": "Bottino", "boss_ucciso": "Boss ucciso", "wipe_ripetuti": "Wipe ripetuti", "fuga": "Fuga",
    "salvataggio": "Salvataggio", "morte": "Morte", "duello": "Duello", "livello": "Livello",
    "prima_visita": "Prima visita", "spesa": "Spesa", "missione_abbandonata": "Missione abbandonata",
}
QUALITA = {3: "raro", 4: "epico", 5: "leggendario"}
INSIEME = {"gruppo": "nello stesso gruppo",
           "stessa_zona": "solo stessa zona e stessa ora (nessun dato di gruppo: forse non erano insieme)"}


# --- letture difensive -------------------------------------------------------

def _int(v) -> int | None:
    if isinstance(v, bool):
        return None
    if isinstance(v, int):
        return v
    if isinstance(v, float) and math.isfinite(v) and abs(v) < 1e15:
        return int(v)
    return None


def _str(v) -> str | None:
    return v if isinstance(v, str) and v else None


def _lista(v) -> list:
    return v if isinstance(v, list) else []


def _dict(v) -> dict:
    return v if isinstance(v, dict) else {}


def _zona(e: dict) -> str | None:
    """Chiave di zona per le unioni: istanza o zona."""
    return _str(e.get("inst")) or _str(e.get("z"))


def _dove(e: dict) -> str:
    inst = _str(e.get("inst"))
    if inst:
        return inst
    z, s = _str(e.get("z")), _str(e.get("s"))
    if z and s and s != z:
        return f"{z}, {s}"
    return z or s or "luogo sconosciuto"


def _nome_base(v) -> str | None:
    """Nome del personaggio senza reame, minuscolo (per confronti)."""
    n = _str(v)
    return n.split("-", 1)[0].lower() if n else None


def _nomi_conta(v, limite: int = 4) -> list[str]:
    """[{name, n}] -> ["Nome x3", ...]"""
    out = []
    for x in _lista(v):
        x = _dict(x)
        nome = _str(x.get("name"))
        if not nome:
            continue
        n = _int(x.get("n"))
        out.append(f"{nome} x{n}" if n and n > 1 else nome)
    return out[:limite]


def _boss_key(e: dict) -> str | None:
    i = _int(e.get("id"))
    return str(i) if i is not None else _str(e.get("name"))


def _finestra(e: dict) -> tuple[int, int]:
    t = _int(e.get("t"))
    da, a = _int(e.get("from")), _int(e.get("to"))
    return (da if da is not None else t, a if a is not None else t)


# --- cronologia per giocatore -------------------------------------------------

def _cronologia(sessioni) -> dict[str, dict]:
    """giocatore -> {eventi, episodi [(ep, char)], char, char_l, gruppi [(t, nomi)]}."""
    out: dict[str, dict] = {}
    for s in _lista(sessioni):
        try:
            if not isinstance(s, dict):
                continue
            g = _str(s.get("giocatore"))
            se = _dict(s.get("sessione"))
            if not g:
                continue
            p = out.setdefault(g, {"eventi": [], "episodi": [], "char": set(), "char_l": set(), "gruppi": []})
            char = _str(se.get("char"))
            if char:
                p["char"].add(char)
                p["char_l"].add(_nome_base(char))
            for e in _lista(se.get("events")):
                if isinstance(e, dict) and _int(e.get("t")) is not None and _str(e.get("k")):
                    p["eventi"].append(e)
                    m = e.get("members")
                    if m == {}:
                        m = []  # tabella Lua vuota = gruppo vuoto
                    if e.get("k") == "group" and isinstance(m, list):
                        p["gruppi"].append((_int(e["t"]), {n for n in map(_nome_base, m) if n}))
            for ep in _lista(s.get("combattimenti")):
                if isinstance(ep, dict) and _int(ep.get("to")) is not None:
                    p["episodi"].append((ep, char))
        except SALTA:
            continue
    for p in out.values():
        p["eventi"].sort(key=lambda e: _int(e["t"]))
        p["episodi"].sort(key=lambda x: _int(x[0]["to"]))
        p["gruppi"].sort(key=lambda x: x[0])
    return dict(sorted(out.items()))


def _membri(p: dict, t: int) -> set:
    """Membri dell'ultimo gruppo del giocatore a t (vuoto se nessun evento di gruppo prima)."""
    m: set = set()
    for tg, nomi in p["gruppi"]:
        if tg > t:
            break
        m = nomi
    return m


def _insieme(pa: dict, ta: int, pb: dict, tb: int) -> str | None:
    """"gruppo" se erano nello stesso gruppo, "nodati" se nessuno dei due ha dati di gruppo."""
    ha, hb = bool(pa["gruppi"]), bool(pb["gruppi"])
    if not ha and not hb:
        return "nodati"
    a_b = ha and bool(_membri(pa, ta) & pb["char_l"])
    b_a = hb and bool(_membri(pb, tb) & pa["char_l"])
    ok = (a_b and b_a) if (ha and hb) else (a_b or b_a)
    return "gruppo" if ok else None


def _di_tipo(p: dict, k: str) -> list[dict]:
    return [e for e in p["eventi"] if e.get("k") == k]


def _evento_a(p: dict, t: int, k: str | None = None) -> dict | None:
    """Evento del tipo `k` con t esatto, altrimenti l'ultimo evento con un luogo prima di t."""
    if k:
        for e in p["eventi"]:
            if e.get("k") == k and _int(e.get("t")) == t:
                return e
    prima = None
    for e in p["eventi"]:
        if _int(e["t"]) > t:
            break
        if _zona(e):
            prima = e
    return prima


def _incontri(p: dict) -> list[dict]:
    """Incontri validi in ordine: {e, key, nome, ok}."""
    out = []
    for e in _di_tipo(p, "encounter"):
        key, ok = _boss_key(e), e.get("ok")
        if key is None or not isinstance(ok, bool):
            continue
        out.append({"e": e, "key": key, "nome": _str(e.get("name")) or key, "ok": ok})
    return out


def _boss_del_bottino(e: dict, uccisi: list[dict]) -> tuple[dict | None, bool]:
    """(uccisione da cui viene il bottino o None, presunta: solo per vicinanza nel tempo)."""
    t = _int(e.get("t"))
    src = _str(e.get("src"))
    if src:
        stessi = [u for u in uccisi if u["nome"] == src]
        prima = [u for u in stessi if _int(u["e"]["t"]) <= t]
        scelti = prima or stessi
        return (scelti[-1] if scelti else None), False
    if "src" in e:
        return None, False
    for u in reversed(uccisi):
        dt = t - _int(u["e"]["t"])
        if 0 <= dt <= DROP_BOSS_S and _zona(u["e"]) == _zona(e):
            return u, True
    return None, False


def _morto_dopo(p: dict, t: int) -> bool:
    for e in _di_tipo(p, "death"):
        if t <= _int(e["t"]) <= t + DOPO_MORTE_S:
            return True
    for ep, _ in p["episodi"]:
        if ep.get("kind") == "death" and t <= _int(ep["to"]) <= t + DOPO_MORTE_S:
            return True
    return False


def _cand(tipo, punti, t, dove, g, fatti, perche, zona=None, fin=None) -> dict:
    return {"tipo": tipo, "punti": punti, "quando": t, "dove": dove, "chi": [g],
            "fatti": fatti, "perche": perche, "_g": g, "_zona": zona, "_fin": fin or (t, t)}


# --- generatori di candidati --------------------------------------------------

def _da_boss(g: str, p: dict, cont: dict) -> list[dict]:
    out = []
    prec = _dict(cont.get("boss"))
    insegui = _dict(cont.get("inseguiti"))
    wipe: dict[str, list[dict]] = {}
    wipe_oggi: dict[str, int] = {}
    oggi: dict[str, int] = {}
    uccisi_oggi: dict[str, int] = {}
    uccisi: list[dict] = []
    for inc in _incontri(p):
        e, key = inc["e"], inc["key"]
        oggi[key] = oggi.get(key, 0) + 1
        if not inc["ok"]:
            wipe.setdefault(key, []).append(e)
            wipe_oggi[key] = wipe_oggi.get(key, 0) + 1
            continue
        n_wipe = len(wipe.pop(key, []))
        uccisi_oggi[key] = uccisi_oggi.get(key, 0) + 1
        inc = dict(inc, n_uccisione=uccisi_oggi[key])
        uccisi.append(inc)
        fatti = {"boss": inc["nome"], "boss_id": key, "tentativi": n_wipe + 1}
        prima = _int(_dict(prec.get(f"{g}|{key}")).get("tentativi")) or 0
        totale = prima + oggi[key]
        if n_wipe:
            perche = f"{inc['nome']} abbattuto dopo {n_wipe} wipe ({n_wipe + 1} tentativi)"
            if prima > 0:
                perche += f", {totale}° tentativo in tutto"
        elif totale == 1:
            perche = f"{inc['nome']} abbattuto al primo tentativo"
        elif not wipe_oggi.get(key):
            perche = f"{inc['nome']} abbattuto senza wipe oggi, al {totale}° tentativo in tutto"
        else:
            perche = f"{inc['nome']} abbattuto di nuovo senza altri wipe, al {totale}° tentativo in tutto"
        if prima > 0:
            fatti["tentativi_totali"] = totale
        size = _int(e.get("size"))
        if size:
            fatti["gruppo"] = size
        out.append(_cand("boss_ucciso", 50 + 10 * n_wipe, _int(e["t"]), _dove(e), g, fatti, perche,
                         _zona(e), _finestra(e)))
    for key, ws in wipe.items():
        if len(ws) >= 3:
            e = ws[-1]
            nome = _str(e.get("name")) or key
            fatti = {"boss": nome, "boss_id": key, "wipe": len(ws)}
            prima = _int(_dict(prec.get(f"{g}|{key}")).get("tentativi")) or 0
            perche = f"{len(ws)} wipe su {nome} senza riuscire ad abbatterlo"
            if prima > 0:
                fatti["tentativi_totali"] = prima + oggi[key]
                perche += f" ({prima + oggi[key]} tentativi in tutto)"
            fin = (_finestra(ws[0])[0], _finestra(ws[-1])[1])
            out.append(_cand("wipe_ripetuti", 40, _int(e["t"]), _dove(e), g, fatti, perche, _zona(e), fin))
    # bottino
    for e in _di_tipo(p, "loot"):
        q = _int(e.get("q"))
        if q is None or q < 3:
            continue
        boss, presunta = _boss_del_bottino(e, uccisi)
        punti = 90 if q >= 5 else 60 if q == 4 else 30 if boss else 0
        if not punti:
            continue
        item = _str(e.get("item")) or "oggetto senza nome"
        fatti = {"oggetto": item, "qualita": QUALITA.get(q, str(q))}
        src = _str(e.get("src"))
        fin = None
        if boss:
            fatti["boss"] = boss["nome"]
            fatti["boss_id"] = boss["key"]
            if presunta:
                fatti["fonte_presunta"] = True
            fin = _finestra(boss["e"])
            if q >= 4:
                prima = _int(_dict(insegui.get(f"{g}|{boss['key']}")).get("uccisioni")) or 0
                n = prima + boss["n_uccisione"]
                if n > 1:
                    fatti["uccisione_n"] = n
        elif src == "fishing":
            fatti["fonte"] = "pesca"
        elif src:
            fatti["fonte"] = src
        perche = f"{item} ({fatti['qualita']})"
        if boss:
            perche += f", probabilmente da {boss['nome']}" if presunta else f" da {boss['nome']}"
            if fatti.get("uccisione_n"):
                perche += f", arrivato alla {fatti['uccisione_n']}ª uccisione"
        elif src == "fishing":
            perche += " pescato dall'acqua"
        elif src:
            perche += f" da {src}"
        out.append(_cand("drop", punti, _int(e["t"]), _dove(e), g, fatti, perche, _zona(e), fin))
    return out


def _guaritori(ep: dict, char: str | None) -> list[str]:
    out = []
    for h in _lista(ep.get("healers")):
        h = _dict(h)
        nome, amount = _str(h.get("name")), _int(h.get("amount"))
        if nome and nome != char and (amount is None or amount > 0) and nome not in out:
            out.append(nome)
    return out


def _da_combattimenti(g: str, p: dict) -> list[dict]:
    out = []
    pesche = _di_tipo(p, "fishing")
    for ep, char in p["episodi"]:
        if ep.get("kind") != "combat":
            continue
        da, a = _int(ep.get("from")), _int(ep["to"])
        ev = _evento_a(p, a, "combat")
        minhp = _int(ep.get("minHp"))
        if minhp is None and ev is not None and ev.get("k") == "combat":
            minhp = _int(ev.get("minHp"))
        if minhp is None or _morto_dopo(p, a):
            continue
        luogo = ev or {}
        nemici = _int(ep.get("enemyCount")) or 0
        nomi = _nomi_conta(ep.get("enemies"))
        cure = _guaritori(ep, char)
        fin = (da if da is not None else a, a)
        if nemici >= 4 and minhp <= 20:
            fatti = {"nemici": nemici, "vita_minima": minhp, "pesca": False}
            if nomi:
                fatti["nemici_nomi"] = nomi
            inizio = fin[0]
            for f in pesche:
                fine = _int(f.get("to"))
                fine = fine if fine is not None else _int(f["t"])
                if inizio - PESCA_S <= fine <= inizio:
                    fatti["pesca"] = True
                    catture = [c for c in _lista(f.get("catches")) if _str(c)]
                    if catture:
                        fatti["catture"] = catture[:5]
            if cure:
                fatti["guaritori"] = cure
            perche = f"Fuga da {nemici} nemici"
            if nomi:
                perche += f" ({', '.join(nomi[:2])})"
            perche += f" con la vita al {minhp}%"
            if fatti["pesca"]:
                perche += ", subito dopo aver pescato"
            out.append(_cand("fuga", 55 + (15 if fatti["pesca"] else 0), a, _dove(luogo), g, fatti,
                             perche, _zona(luogo), fin))
        elif minhp <= 10 and cure:  # una fuga tiene gia' i guaritori
            fatti = {"vita_minima": minhp, "guaritori": cure}
            if nemici:
                fatti["nemici"] = nemici
            if nomi:
                fatti["nemici_nomi"] = nomi
            perche = f"Salvato con la vita al {minhp}% dalle cure di {', '.join(cure[:3])}"
            out.append(_cand("salvataggio", 45, a, _dove(luogo), g, fatti, perche, _zona(luogo), fin))
    return out


def _episodio_morte(p: dict, t: int, usati: set) -> dict | None:
    for i, (ep, _) in enumerate(p["episodi"]):
        if i not in usati and ep.get("kind") == "death" and abs(_int(ep["to"]) - t) <= 1:
            usati.add(i)
            return ep
    return None


def _da_morti(g: str, p: dict) -> list[dict]:
    out = []
    usati: set = set()
    per_zona: dict[str, list[tuple[dict, dict]]] = {}
    for e in _di_tipo(p, "death"):
        t = _int(e["t"])
        ep = _episodio_morte(p, t, usati) or {}
        amb = {_str(_dict(x).get("name")) for x in _lista(ep.get("environmental"))}
        causa = ("annegamento" if e.get("breath") is True or "Drowning" in amb
                 else "caduta" if "Falling" in amb else None)
        killer = _dict(ep.get("killedBy"))
        chi_uccide = _str(killer.get("who"))
        if causa:
            fatti = {"causa": causa}
            perche = "Morto annegato" if causa == "annegamento" else "Morto per una caduta"
            out.append(_cand("morte", 40, t, _dove(e), g, fatti, perche, _zona(e)))
            continue
        per_zona.setdefault(_zona(e) or "", []).append((e, {"uccisore": chi_uccide} if chi_uccide else {}))
    for zona, morti in per_zona.items():
        if len(morti) >= 3:
            e = morti[0][0]
            uccisori = []
            for _, f in morti:
                if f.get("uccisore") and f["uccisore"] not in uccisori:
                    uccisori.append(f["uccisore"])
            fatti = {"morti": len(morti)}
            if uccisori:
                fatti["uccisori"] = uccisori[:4]
            perche = f"{len(morti)} morti a {_dove(e)} nella stessa giornata"
            if uccisori:
                perche += f" (per mano di {', '.join(uccisori[:2])})"
            fin = (_int(e["t"]), _int(morti[-1][0]["t"]))
            out.append(_cand("morte", 35, _int(e["t"]), _dove(e), g, fatti, perche, zona or None, fin))
        else:
            for e, f in morti:
                perche = f"Caduto per mano di {f['uccisore']}" if f.get("uccisore") else "Una morte in battaglia"
                out.append(_cand("morte", 10, _int(e["t"]), _dove(e), g, dict(f), perche, zona or None))
    return out


def _da_eventi_semplici(g: str, p: dict) -> list[dict]:
    out = []
    for e in p["eventi"]:
        k, t = e.get("k"), _int(e["t"])
        if k == "level":
            lv = _int(e.get("level"))
            if lv is not None and lv > 0 and (lv == 60 or lv % 10 == 0):
                perche = "Raggiunto il livello 60, il massimo!" if lv == 60 else f"Raggiunto il livello {lv}"
                out.append(_cand("livello", 70 if lv == 60 else 20, t, _dove(e), g, {"livello": lv},
                                 perche, _zona(e)))
        elif k == "zone" and e.get("first") is True:
            out.append(_cand("prima_visita", 5, t, _dove(e), g, {}, f"Prima volta a {_dove(e)}", _zona(e)))
        elif k == "spend":
            amount = _int(e.get("amount"))
            if amount is not None and amount >= ORO_FOLLE:
                oro = amount // 10000
                fatti = {"oro": oro}
                dove_spesa = {"merchant": "dal mercante", "auction": "all'asta", "trainer": "dall'addestratore"}
                w = _str(e.get("where"))
                if w:
                    fatti["dove_spesa"] = w
                perche = f"Spesi {oro} oro in un colpo solo" + (f" {dove_spesa[w]}" if w in dove_spesa else "")
                out.append(_cand("spesa", 25, t, _dove(e), g, fatti, perche, _zona(e)))
        elif k == "quest" and e.get("action") == "abandon":
            titolo = _str(e.get("title"))
            fatti = {"missione": titolo} if titolo else {}
            perche = f"Missione «{titolo}» abbandonata" if titolo else "Una missione abbandonata"
            out.append(_cand("missione_abbandonata", 10, t, _dove(e), g, fatti, perche, _zona(e)))
    return out


def _duelli(cron: dict[str, dict]) -> list[dict]:
    per_char = {}
    for g, p in cron.items():
        for c in sorted(p["char"]):
            per_char.setdefault(_nome_base(c), g)
    out = []
    for g, p in cron.items():
        for e in _di_tipo(p, "duel"):
            try:
                v, s = per_char.get(_nome_base(e.get("winner"))), per_char.get(_nome_base(e.get("loser")))
                if not v or not s or v == s:
                    continue
                t = _int(e["t"])
                if any(set(d["chi"]) == {v, s} and abs(d["quando"] - t) <= 60 for d in out):
                    continue
                fatti = {"vincitore": v, "sconfitto": s}
                how = _str(e.get("how"))
                if how:
                    fatti["come"] = how
                perche = f"Duello tra membri: {v} batte {s}" + (" (l'avversario si ritira)" if how == "retreat" else "")
                c = _cand("duello", 30, t, _dove(e), v, fatti, perche, _zona(e))
                c["chi"] = sorted({v, s})
                out.append(c)
            except SALTA:
                continue
    return out


# --- unione degli episodi condivisi -----------------------------------------

def _legame(gruppo: list[dict], c: dict, cron: dict) -> str | None:
    """Come `c` si unisce al gruppo: "gruppo", "stessa_zona" o None (non si unisce)."""
    if gruppo[0]["tipo"] != c["tipo"] or c["tipo"] == "duello":
        return None
    if set(c["chi"]) & {n for x in gruppo for n in x["chi"]}:
        return None
    for x in gruppo:
        rel = _insieme(cron[x["_g"]], x["quando"], cron[c["_g"]], c["quando"])
        vicini = abs(x["quando"] - c["quando"]) <= UNIONE_S
        bx, bc = x["fatti"].get("boss_id"), c["fatti"].get("boss_id")
        if bx is not None or bc is not None:
            if bx != bc:
                continue  # boss diversi (o boss e non boss): mai uniti
            sovrapposti = x["_fin"][0] <= c["_fin"][1] and c["_fin"][0] <= x["_fin"][1]
            if not (sovrapposti or vicini):
                continue
            if rel == "gruppo":
                return "gruppo"
            if rel == "nodati" and x["_zona"] and x["_zona"] == c["_zona"]:
                return "stessa_zona"
            continue
        if not vicini:
            continue
        if rel == "gruppo":
            return "gruppo"
        if rel == "nodati" and x["_zona"] and x["_zona"] == c["_zona"]:
            return "stessa_zona"
    return None


def _per_giocatore(gr: list[dict]) -> dict:
    """Fatti per giocatore: i numeri e cio' che cambia tra uno e l'altro."""
    chiavi = sorted({k for x in gr for k in x["fatti"] if k != "boss_id"})
    diverse = {k for k in chiavi
               if len({json.dumps(x["fatti"].get(k), sort_keys=True, default=str) for x in gr}) > 1}
    out = {}
    for x in gr:
        f = {k: v for k, v in x["fatti"].items() if k in chiavi
             and (k in diverse or (isinstance(v, int) and not isinstance(v, bool)))}
        if f:
            out[", ".join(x["chi"])] = f
    return dict(sorted(out.items()))


def _unisci(cand: list[dict], cron: dict) -> list[dict]:
    gruppi: list[tuple[list[dict], list[str]]] = []
    ordine = lambda c: (c["quando"], c["tipo"], c["chi"], -c["punti"], c["perche"],
                        json.dumps(c["fatti"], sort_keys=True, default=str))
    for c in sorted(cand, key=ordine):
        for gr, legami in gruppi:
            rel = _legame(gr, c, cron)
            if rel:
                gr.append(c)
                legami.append(rel)
                break
        else:
            gruppi.append(([c], []))
    out = []
    for gr, legami in gruppi:
        base = min(gr, key=lambda x: (-x["punti"],) + ordine(x))
        chi = sorted({n for x in gr for n in x["chi"]})
        fatti = dict(base["fatti"])
        perche = base["perche"]
        if len(gr) > 1:
            fatti["insieme"] = "gruppo" if all(r == "gruppo" for r in legami) else "stessa_zona"
            pg = _per_giocatore(gr)
            if pg:
                fatti["per_giocatore"] = pg
            if base["tipo"] == "morte" and all(_int(x["fatti"].get("morti")) for x in gr):
                fatti["morti"] = sum(x["fatti"]["morti"] for x in gr)
                conti = ", ".join(str(x["chi"][0]) + " " + str(x["fatti"]["morti"])
                                  for x in sorted(gr, key=lambda y: y["chi"]))
                perche = f"{fatti['morti']} morti a {base['dove']} tra {', '.join(chi)} ({conti})"
        out.append({"tipo": base["tipo"], "punti": max(x["punti"] for x in gr) + 10 * (len(chi) - len(base["chi"])),
                    "quando": min(x["quando"] for x in gr), "dove": base["dove"], "chi": chi,
                    "fatti": fatti, "perche": perche})
    return out


# --- interfaccia ---------------------------------------------------------------

def candidati(sessioni: list[dict], contatori: dict) -> list[dict]:
    """Momenti candidati della giornata, ordinati per punti (poi quando, tipo, ...)."""
    cont = _dict(contatori)
    cron = _cronologia(sessioni)
    grezzi: list[dict] = []
    for g, p in cron.items():
        for fonte in (lambda: _da_boss(g, p, cont), lambda: _da_combattimenti(g, p),
                      lambda: _da_morti(g, p), lambda: _da_eventi_semplici(g, p)):
            try:
                grezzi.extend(fonte())
            except SALTA:
                continue  # fonte malformata: si salta
    try:
        grezzi.extend(_duelli(cron))
    except SALTA:
        pass
    out = _unisci(grezzi, cron)
    out.sort(key=lambda c: (-c["punti"], c["quando"], c["tipo"], c["chi"], c["dove"], c["perche"],
                            json.dumps(c["fatti"], sort_keys=True, default=str)))
    return out


def aggiorna_contatori(contatori: dict, sessioni) -> dict:
    """Nuovi contatori (puri, serializzabili in JSON) con i tentativi della giornata.

    boss["giocatore|boss"] = {nome, tentativi, uccisioni, epico?};
    inseguiti["giocatore|boss"] = {nome, uccisioni}: boss uccisi senza che il drop epico sia arrivato.
    """
    out = copy.deepcopy(_dict(contatori))
    boss = out["boss"] = _dict(out.get("boss"))
    insegui = out["inseguiti"] = _dict(out.get("inseguiti"))
    for g, p in _cronologia(sessioni).items():
        try:
            uccisi = []
            for inc in _incontri(p):
                k = f"{g}|{inc['key']}"
                b = boss[k] = _dict(boss.get(k))
                b["nome"] = inc["nome"]
                b["tentativi"] = (_int(b.get("tentativi")) or 0) + 1
                if inc["ok"]:
                    b["uccisioni"] = (_int(b.get("uccisioni")) or 0) + 1
                    uccisi.append(inc)
                else:
                    b.setdefault("uccisioni", 0)
            epici = {}
            for e in _di_tipo(p, "loot"):
                q = _int(e.get("q"))
                if q is not None and q >= 4:
                    u, _ = _boss_del_bottino(e, uccisi)
                    if u is not None:
                        epici.setdefault(id(u), _str(e.get("item")) or "oggetto senza nome")
            for u in uccisi:
                k = f"{g}|{u['key']}"
                if id(u) in epici:
                    boss[k]["epico"] = epici[id(u)]
                    insegui.pop(k, None)
                elif "epico" not in boss[k]:
                    i = insegui[k] = _dict(insegui.get(k))
                    i["nome"] = u["nome"]
                    i["uccisioni"] = (_int(i.get("uccisioni")) or 0) + 1
        except SALTA:
            continue
    return out


def _ora(t: int, fuso: str) -> str:
    try:
        return datetime.fromtimestamp(t, ZoneInfo(fuso)).strftime("%H:%M")
    except (OverflowError, OSError, ValueError):
        return "?"


def _valore(v) -> str:
    if isinstance(v, bool):
        return "sì" if v else "no"
    if isinstance(v, list):
        return ", ".join(_valore(x) for x in v)
    if isinstance(v, dict):
        return "; ".join(f"{pulito(k, 40)} {_valore(x)}" for k, x in v.items())
    return pulito(v)


def _riga_fatti(fatti: dict) -> str:
    parti = []
    if "in_ritardo" in fatti:  # sempre per primo: il taglio a 300 caratteri non lo perde
        parti.append(f"in ritardo: {_valore(fatti['in_ritardo'])}")
    for k, v in fatti.items():
        if k in ("boss_id", "altri", "insieme", "per_giocatore", "in_ritardo"):
            continue
        parti.append(f"{pulito(str(k).replace('_', ' '), 40)}: {_valore(v)}")
    s = "; ".join(parti)
    return s if len(s) <= 300 else s[:297] + "..."


def _come_lista(v) -> list:
    if v is None:
        return []
    if isinstance(v, str):
        return [v]
    return list(v)


def _nomi(giocatori) -> str:
    if isinstance(giocatori, dict):
        nomi = [f"{pulito(k)} ({pulito(v, 200)})" if v else pulito(k) for k, v in giocatori.items()]
    else:
        nomi = [pulito(x) for x in _come_lista(giocatori)]
    return ", ".join(nomi)


def riassunto_md(giorno: str, candidati, avvisi, giocatori, fuso: str = "Europe/Rome") -> str:
    """Riassunto compatto della giornata per Claude: giocatori, candidati numerati, avvisi."""
    # ogni testo dai pacchetti passa da `pulito`: una riga, corto, mai un'intestazione
    righe = [f"# Giornata di gioco {giorno}", "",
             f"Giocatori: {_nomi(giocatori) or 'nessuno'}", ""]
    cand = [c for c in _come_lista(candidati) if isinstance(c, dict)]
    righe.append(f"## Momenti candidati ({len(cand)})")
    righe.append("")
    if not cand:
        righe.append("Nessun momento notevole.")
    for i, c in enumerate(cand[:MAX_RIASSUNTO], 1):
        tipo = NOMI_TIPO.get(c.get("tipo")) or pulito(c.get("tipo"), 40)
        chi = ", ".join(pulito(x) for x in _lista(c.get("chi")))
        quando = _int(c.get("quando"))
        ora = _ora(quando, fuso) if quando is not None else "?"
        righe.append(f"{i}. **{tipo}** ({_int(c.get('punti'))} punti) — {ora}, {pulito(c.get('dove'))} — {chi}")
        righe.append(f"   {pulito(c.get('perche'), 300)}")
        fatti = _dict(c.get("fatti"))
        rf = _riga_fatti(fatti)
        if rf:
            righe.append(f"   Fatti: {rf}")
        ins = fatti.get("insieme")
        if ins in INSIEME:
            righe.append(f"   Insieme: {INSIEME[ins]}")
        pg = _dict(fatti.get("per_giocatore"))
        if pg:
            parti = [f"{pulito(n)} ({_riga_fatti(_dict(f))})" for n, f in list(pg.items())[:5]]
            righe.append(f"   Per giocatore: {'; '.join(parti)}"[:400])
    if len(cand) > MAX_RIASSUNTO:
        righe.append(f"… e altri {len(cand) - MAX_RIASSUNTO} candidati minori omessi.")
    righe += ["", "## Avvisi", ""]
    avv = [pulito(a, 200) for a in _come_lista(avvisi)]
    righe += [f"- {a}" for a in avv] if avv else ["Nessun avviso."]
    return "\n".join(righe) + "\n"
