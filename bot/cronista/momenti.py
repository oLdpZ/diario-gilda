"""Momenti candidati di una giornata di gioco, contatori tra i giorni e riassunto per Claude."""
from __future__ import annotations

import copy
import math
from datetime import datetime
from zoneinfo import ZoneInfo

UNIONE_S = 300          # episodi condivisi: stessa zona entro 5 minuti
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


# --- cronologia per giocatore -------------------------------------------------

def _cronologia(sessioni) -> dict[str, dict]:
    """giocatore -> {"eventi": [...] ordinati, "episodi": [(ep, char)], "char": set}."""
    out: dict[str, dict] = {}
    for s in _lista(sessioni):
        try:
            if not isinstance(s, dict):
                continue
            g = _str(s.get("giocatore"))
            se = _dict(s.get("sessione"))
            if not g:
                continue
            p = out.setdefault(g, {"eventi": [], "episodi": [], "char": set()})
            char = _str(se.get("char"))
            if char:
                p["char"].add(char)
            for e in _lista(se.get("events")):
                if isinstance(e, dict) and _int(e.get("t")) is not None and _str(e.get("k")):
                    p["eventi"].append(e)
            for ep in _lista(s.get("combattimenti")):
                if isinstance(ep, dict) and _int(ep.get("to")) is not None:
                    p["episodi"].append((ep, char))
        except SALTA:
            continue
    for p in out.values():
        p["eventi"].sort(key=lambda e: _int(e["t"]))
        p["episodi"].sort(key=lambda x: _int(x[0]["to"]))
    return dict(sorted(out.items()))


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


def _boss_del_bottino(e: dict, uccisi: list[dict]) -> dict | None:
    """Uccisione (da `_incontri`) da cui viene il bottino, se c'e'."""
    t = _int(e.get("t"))
    src = _str(e.get("src"))
    if src:
        stessi = [u for u in uccisi if u["nome"] == src]
        prima = [u for u in stessi if _int(u["e"]["t"]) <= t]
        scelti = prima or stessi
        return scelti[-1] if scelti else None
    if "src" in e:
        return None
    for u in reversed(uccisi):
        dt = t - _int(u["e"]["t"])
        if 0 <= dt <= DROP_BOSS_S and _zona(u["e"]) == _zona(e):
            return u
    return None


def _morto_dopo(p: dict, t: int) -> bool:
    for e in _di_tipo(p, "death"):
        if t <= _int(e["t"]) <= t + DOPO_MORTE_S:
            return True
    for ep, _ in p["episodi"]:
        if ep.get("kind") == "death" and t <= _int(ep["to"]) <= t + DOPO_MORTE_S:
            return True
    return False


def _cand(tipo, punti, e_or_t, dove, g, fatti, perche, zona=None) -> dict:
    return {"tipo": tipo, "punti": punti, "quando": e_or_t, "dove": dove, "chi": [g],
            "fatti": fatti, "perche": perche, "_zona": zona}


# --- generatori di candidati --------------------------------------------------

def _da_boss(g: str, p: dict, cont: dict) -> list[dict]:
    out = []
    prec = _dict(cont.get("boss"))
    insegui = _dict(cont.get("inseguiti"))
    wipe: dict[str, list[dict]] = {}
    oggi: dict[str, int] = {}
    uccisi_oggi: dict[str, int] = {}
    uccisi: list[dict] = []
    for inc in _incontri(p):
        e, key = inc["e"], inc["key"]
        oggi[key] = oggi.get(key, 0) + 1
        if not inc["ok"]:
            wipe.setdefault(key, []).append(e)
            continue
        n_wipe = len(wipe.pop(key, []))
        uccisi_oggi[key] = uccisi_oggi.get(key, 0) + 1
        inc = dict(inc, n_uccisione=uccisi_oggi[key])
        uccisi.append(inc)
        fatti = {"boss": inc["nome"], "boss_id": key, "tentativi": n_wipe + 1}
        prima = _int(_dict(prec.get(f"{g}|{key}")).get("tentativi")) or 0
        perche = (f"{inc['nome']} abbattuto al primo colpo" if n_wipe == 0
                  else f"{inc['nome']} abbattuto dopo {n_wipe} wipe ({n_wipe + 1} tentativi)")
        if prima > 0:
            fatti["tentativi_totali"] = prima + oggi[key]
            perche += f", {prima + oggi[key]}° tentativo in tutto"
        size = _int(e.get("size"))
        if size:
            fatti["gruppo"] = size
        out.append(_cand("boss_ucciso", 50 + 10 * n_wipe, _int(e["t"]), _dove(e), g, fatti, perche, _zona(e)))
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
            out.append(_cand("wipe_ripetuti", 40, _int(e["t"]), _dove(e), g, fatti, perche, _zona(e)))
    # bottino
    for e in _di_tipo(p, "loot"):
        q = _int(e.get("q"))
        if q is None or q < 3:
            continue
        boss = _boss_del_bottino(e, uccisi)
        punti = 90 if q >= 5 else 60 if q == 4 else 30 if boss else 0
        if not punti:
            continue
        item = _str(e.get("item")) or "oggetto senza nome"
        fatti = {"oggetto": item, "qualita": QUALITA.get(q, str(q))}
        src = _str(e.get("src"))
        if boss:
            fatti["boss"] = boss["nome"]
            fatti["boss_id"] = boss["key"]
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
            perche += f" da {boss['nome']}"
            if fatti.get("uccisione_n"):
                perche += f", arrivato alla {fatti['uccisione_n']}ª uccisione"
        elif src == "fishing":
            perche += " pescato dall'acqua"
        elif src:
            perche += f" da {src}"
        out.append(_cand("drop", punti, _int(e["t"]), _dove(e), g, fatti, perche, _zona(e)))
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
        if nemici >= 4 and minhp <= 20:
            fatti = {"nemici": nemici, "vita_minima": minhp, "pesca": False}
            if nomi:
                fatti["nemici_nomi"] = nomi
            inizio = da if da is not None else a
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
                             perche, _zona(luogo)))
        if minhp <= 10 and cure:
            fatti = {"vita_minima": minhp, "guaritori": cure}
            if nemici:
                fatti["nemici"] = nemici
            if nomi:
                fatti["nemici_nomi"] = nomi
            perche = f"Salvato con la vita al {minhp}% dalle cure di {', '.join(cure[:3])}"
            out.append(_cand("salvataggio", 45, a, _dove(luogo), g, fatti, perche, _zona(luogo)))
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
            out.append(_cand("morte", 35, _int(e["t"]), _dove(e), g, fatti, perche, zona or None))
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


def _senza_reame(nome) -> str | None:
    n = _str(nome)
    return n.split("-", 1)[0] if n else None


def _duelli(cron: dict[str, dict]) -> list[dict]:
    per_char = {}
    for g, p in cron.items():
        for c in sorted(p["char"]):
            per_char.setdefault(c, g)
    out = []
    for g, p in cron.items():
        for e in _di_tipo(p, "duel"):
            try:
                v, s = per_char.get(_senza_reame(e.get("winner"))), per_char.get(_senza_reame(e.get("loser")))
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

def _unibili(gruppo: list[dict], c: dict) -> bool:
    if gruppo[0]["tipo"] != c["tipo"]:
        return False
    if set(c["chi"]) & {n for x in gruppo for n in x["chi"]}:
        return False
    for x in gruppo:
        bid = x["fatti"].get("boss_id")
        if bid is not None and bid == c["fatti"].get("boss_id"):
            return True
        if x["_zona"] and x["_zona"] == c["_zona"] and abs(x["quando"] - c["quando"]) <= UNIONE_S:
            return True
    return False


def _unisci(cand: list[dict]) -> list[dict]:
    gruppi: list[list[dict]] = []
    for c in sorted(cand, key=lambda c: (c["quando"], c["tipo"], c["chi"], -c["punti"])):
        for gr in gruppi:
            if _unibili(gr, c):
                gr.append(c)
                break
        else:
            gruppi.append([c])
    out = []
    for gr in gruppi:
        base = min(gr, key=lambda x: (-x["punti"], x["quando"], x["chi"]))
        chi = sorted({n for x in gr for n in x["chi"]})
        fatti = dict(base["fatti"])
        altri = [dict(x["fatti"], chi=x["chi"]) for x in gr if x is not base and x["fatti"] != base["fatti"]]
        if altri:
            fatti["altri"] = altri
        out.append({"tipo": base["tipo"], "punti": max(x["punti"] for x in gr) + 10 * (len(chi) - len(base["chi"])),
                    "quando": min(x["quando"] for x in gr), "dove": base["dove"], "chi": chi,
                    "fatti": fatti, "perche": base["perche"]})
    return out


# --- interfaccia ---------------------------------------------------------------

def candidati(sessioni: list[dict], contatori: dict) -> list[dict]:
    """Momenti candidati della giornata, ordinati per punti (poi quando, poi tipo)."""
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
    out = _unisci(grezzi)
    out.sort(key=lambda c: (-c["punti"], c["quando"], c["tipo"], c["chi"], c["dove"]))
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
                    u = _boss_del_bottino(e, uccisi)
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
        return "; ".join(f"{k} {_valore(x)}" for k, x in v.items())
    return str(v)


def _riga_fatti(fatti: dict) -> str:
    parti = []
    for k, v in fatti.items():
        if k in ("boss_id", "altri"):
            continue
        parti.append(f"{k.replace('_', ' ')}: {_valore(v)}")
    s = "; ".join(parti)
    return s if len(s) <= 300 else s[:297] + "..."


def riassunto_md(giorno: str, candidati, avvisi, giocatori, fuso: str = "Europe/Rome") -> str:
    """Riassunto compatto della giornata per Claude: giocatori, candidati numerati, avvisi."""
    if isinstance(giocatori, dict):
        nomi = [f"{k} ({v})" if v else str(k) for k, v in giocatori.items()]
    else:
        nomi = [str(x) for x in (giocatori or [])]
    righe = [f"# Giornata di gioco {giorno}", "",
             f"Giocatori: {', '.join(nomi) if nomi else 'nessuno'}", ""]
    cand = [c for c in (candidati or []) if isinstance(c, dict)]
    righe.append(f"## Momenti candidati ({len(cand)})")
    righe.append("")
    if not cand:
        righe.append("Nessun momento notevole.")
    for i, c in enumerate(cand[:MAX_RIASSUNTO], 1):
        tipo = NOMI_TIPO.get(c.get("tipo"), str(c.get("tipo")))
        chi = ", ".join(str(x) for x in _lista(c.get("chi")))
        quando = _int(c.get("quando"))
        ora = _ora(quando, fuso) if quando is not None else "?"
        righe.append(f"{i}. **{tipo}** ({c.get('punti')} punti) — {ora}, {c.get('dove')} — {chi}")
        righe.append(f"   {c.get('perche')}")
        fatti = _dict(c.get("fatti"))
        rf = _riga_fatti(fatti)
        if rf:
            righe.append(f"   Fatti: {rf}")
        altri = [a for a in _lista(fatti.get("altri")) if isinstance(a, dict)]
        if altri:
            parti = []
            for a in altri[:4]:
                a = dict(a)
                n = ", ".join(str(x) for x in _lista(a.pop("chi", [])))
                parti.append(f"{n} ({_riga_fatti(a)})")
            righe.append(f"   Anche: {'; '.join(parti)}"[:400])
    if len(cand) > MAX_RIASSUNTO:
        righe.append(f"… e altri {len(cand) - MAX_RIASSUNTO} candidati minori omessi.")
    righe += ["", "## Avvisi", ""]
    avv = [str(a) for a in (avvisi or [])]
    righe += [f"- {a}" for a in avv] if avv else ["Nessun avviso."]
    return "\n".join(righe) + "\n"
