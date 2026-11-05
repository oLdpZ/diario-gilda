"""Bilancio della giornata: il resoconto esatto di ogni personaggio, senza interpretazioni."""
from __future__ import annotations

from . import ingresso
from .momenti import SALTA, _dict, _int, _lista, _str
from .testo import pulito

MAX_MISSIONI = 8
MAX_LUOGHI = 5
MESI = ["gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto",
        "settembre", "ottobre", "novembre", "dicembre"]


def _base(char: str) -> str:
    return char.split("-", 1)[0]


def righe(sessioni: list[dict], tardi=frozenset(), fuso: str = "Europe/Rome",
          inizio: str = "06:00") -> list[dict]:
    """Una voce per (giocatore, personaggio); le sessioni in ritardo hanno la loro voce a parte."""
    gruppi: dict[tuple, dict] = {}
    for s in _lista(sessioni):
        try:
            if not isinstance(s, dict):
                continue
            g = _str(s.get("giocatore"))
            se = _dict(s.get("sessione"))
            if not g:
                continue
            char = _base(_str(se.get("char")) or g)
            ritardo = None
            if id(s) in tardi:
                try:
                    ritardo = ingresso.giornata(_int(se.get("start")), fuso, inizio)
                except SALTA:
                    ritardo = "?"
            gr = gruppi.setdefault((g, char.lower(), ritardo or ""),
                                   {"giocatore": g, "char": char, "ritardo": ritardo, "sess": []})
            gr["sess"].append(s)
        except SALTA:
            continue
    out = []
    for chiave in sorted(gruppi):
        try:
            out.append(_voce(gruppi[chiave]))
        except SALTA:
            continue
    return out


def _voce(gr: dict) -> dict:
    sess = sorted(gr["sess"], key=lambda s: _int(_dict(s.get("sessione")).get("start")) or 0)
    eventi, livello0, n_episodi, nemici = [], None, 0, 0
    for s in sess:
        se = _dict(s.get("sessione"))
        if livello0 is None:
            livello0 = _int(se.get("level"))
        eventi += [(_int(e.get("t")), e) for e in _lista(se.get("events"))
                   if isinstance(e, dict) and _int(e.get("t")) is not None]
        for ep in _lista(s.get("combattimenti")):
            if not isinstance(ep, dict):
                continue
            n_episodi += 1
            for k in _lista(ep.get("kills")):
                nemici += max(_int(_dict(k).get("n")) or 0, 0)
    eventi.sort(key=lambda x: x[0])
    livelli = [lv for lv in (_int(e.get("level")) for _, e in eventi if e.get("k") == "level") if lv]
    da = livello0 if livello0 else (livelli[0] - 1 if livelli else None)
    a = livelli[-1] if livelli else da
    missioni, luoghi, morti = [], [], 0
    for _, e in eventi:
        k = e.get("k")
        if k == "quest" and e.get("action") == "done":
            titolo = _str(e.get("title"))
            titolo = pulito(titolo, 60) if titolo else None
            if titolo and titolo not in missioni:
                missioni.append(titolo)
        elif k == "death":
            morti += 1
        luogo = _str(e.get("s")) or _str(e.get("z"))
        if luogo:
            luogo = pulito(luogo, 60)
            if luogo and luogo not in luoghi:
                luoghi.append(luogo)
    return {"giocatore": gr["giocatore"], "char": gr["char"], "da": da, "a": a, "missioni": missioni,
            "nemici": nemici if n_episodi else None, "morti": morti, "luoghi": luoghi[:MAX_LUOGHI],
            "ritardo": gr["ritardo"]}


def _giorno(data: str) -> str | None:
    try:
        _, m, d = data.split("-")
        return f"{int(d)} {MESI[int(m) - 1]}"
    except (ValueError, IndexError, AttributeError):
        return None


def _riga(v: dict) -> str:
    pezzi = []
    da, a = _int(v.get("da")), _int(v.get("a"))
    if a is not None:
        pezzi.append(f"livello {a}" if da is None or da == a else f"livello {da} → {a}")
    miss = [pulito(m, 60) for m in _lista(v.get("missioni")) if _str(m)]
    if miss:
        elenco = ", ".join(miss[:MAX_MISSIONI])
        if len(miss) > MAX_MISSIONI:
            elenco += f", e altre {len(miss) - MAX_MISSIONI}"
        pezzi.append(f"{len(miss)} missione ({elenco})" if len(miss) == 1
                     else f"{len(miss)} missioni ({elenco})")
    nemici = _int(v.get("nemici"))
    if nemici is not None:
        pezzi.append(f"{nemici} nemico" if nemici == 1 else f"{nemici} nemici")
    morti = _int(v.get("morti")) or 0
    if morti > 0:
        pezzi.append("1 morte" if morti == 1 else f"{morti} morti")
    luoghi = [pulito(x, 60) for x in _lista(v.get("luoghi")) if _str(x)]
    if luoghi:
        pezzi.append(", ".join(luoghi[:MAX_LUOGHI]))
    nome = pulito(_base(_str(v.get("char")) or _str(v.get("giocatore")) or "?"), 40)
    riga = f"**{nome}**" + (" — " + " · ".join(pezzi) if pezzi else "")
    ritardo = _str(v.get("ritardo"))
    if ritardo:
        g = _giorno(ritardo)
        riga += f" *(notizie dal {g})*" if g else " *(notizie in ritardo)*"
    return riga


def md(voci: list[dict]) -> str:
    """Blocco Markdown: titolo e una riga per voce."""
    return "\n".join(["**Bilancio della giornata**", ""]
                     + [_riga(v) for v in _lista(voci) if isinstance(v, dict)]) + "\n"
