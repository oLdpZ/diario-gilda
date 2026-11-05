"""Parti pure della Voce del Saggio: trovare le pagine, pulire il testo, frasi, sottotitoli, bocca."""
from __future__ import annotations

import re
from datetime import datetime, timedelta, timezone

PIEDE = "Il Cronista della Gilda"
INIZIO_BILANCIO = "**Bilancio della giornata**"
PREFISSO_FILE = "cronaca-"
MIN_VOCE = 40          # frasi più corte si leggono insieme alla successiva (evita l'accento straniero)
MAX_SOTTOTITOLO = 90


def _quando(m: dict) -> datetime:
    return datetime.fromisoformat(m["timestamp"].replace("Z", "+00:00"))


def pagine(messaggi: list[dict], io: str) -> list[dict]:
    """Pagine del Cronista nei messaggi (dal più vecchio): {"id", "quando", "titolo", "testo"}.

    Una pagina comincia con un messaggio del bot il cui primo embed ha titolo con «Cronache» e il piede
    del Cronista; i messaggi del bot subito dopo, solo embed senza titolo, ne sono il seguito."""
    out: list[dict] = []
    cur: dict | None = None
    for m in messaggi:
        emb = m.get("embeds") or []
        del_bot = str((m.get("author") or {}).get("id")) == str(io)
        primo = emb[0] if emb else {}
        if del_bot and emb and "Cronache" in (primo.get("title") or "") \
                and (primo.get("footer") or {}).get("text") == PIEDE:
            cur = {"id": m["id"], "quando": _quando(m), "titolo": primo["title"],
                   "parti": [e.get("description", "") for e in emb]}
            out.append(cur)
        elif del_bot and emb and cur is not None and not any(e.get("title") for e in emb) \
                and not m.get("attachments"):
            cur["parti"].extend(e.get("description", "") for e in emb)
        else:
            cur = None
    for p in out:
        p["testo"] = "\n\n".join(x for x in p.pop("parti") if x)
    return out


def narrate(messaggi: list[dict], io: str) -> set[str]:
    """Id delle pagine che hanno già il loro video (allegato cronaca-<id>.mp4 del bot)."""
    fatte = set()
    for m in messaggi:
        if str((m.get("author") or {}).get("id")) != str(io):
            continue
        for a in m.get("attachments") or []:
            n = a.get("filename", "")
            if n.startswith(PREFISSO_FILE) and n.endswith(".mp4"):
                fatte.add(n[len(PREFISSO_FILE):-4])
    return fatte


def da_narrare(messaggi: list[dict], io: str, ora: datetime, ore: float = 20) -> list[dict]:
    """Pagine recenti (ultime `ore`) ancora senza video, dalla più vecchia."""
    fatte = narrate(messaggi, io)
    limite = ora - timedelta(hours=ore)
    return [p for p in pagine(messaggi, io) if p["quando"] >= limite and p["id"] not in fatte]


def pulisci(testo: str) -> str:
    """Testo da leggere: senza bilancio, senza segni di Markdown, senza caratteri invisibili."""
    i = testo.find(INIZIO_BILANCIO)
    if i >= 0:
        testo = testo[:i]
    testo = testo.replace("​", "")
    testo = re.sub(r"^[ \t]*(#+|>|[-*•][ \t])[ \t]*", "", testo, flags=re.M)
    testo = re.sub(r"(\*\*|__|\*|_|`|~~)", "", testo)
    testo = re.sub(r"[ \t]+", " ", testo)
    return re.sub(r"\n{3,}", "\n\n", testo).strip()


_FRASE = re.compile(r".+?(?:[.!?…]+[»”\"')]*(?=\s|$)|$)", re.S)


def frasi(paragrafo: str) -> list[str]:
    p = " ".join(paragrafo.split())
    return [f.strip() for f in _FRASE.findall(p) if f.strip()]


def blocchi(titolo: str, testo: str) -> list[list[str]]:
    """Paragrafi -> blocchi da leggere in una sola chiamata (frasi corte unite alla successiva).
    Il titolo è il primo paragrafo («Cronache della Gilda. Giorno 2, …»)."""
    paragrafi = [titolo.replace(" — ", ". ").strip().rstrip(".") + "."]
    paragrafi += [p for p in pulisci(testo).split("\n\n") if p.strip()]
    out = []
    for par in paragrafi:
        bl, cur = [], ""
        for f in frasi(par):
            cur = f"{cur} {f}".strip()
            if len(cur) >= MIN_VOCE:
                bl.append(cur); cur = ""
        if cur:
            if bl and len(cur) < MIN_VOCE:
                bl[-1] = f"{bl[-1]} {cur}"
            else:
                bl.append(cur)
        if bl:
            out.append(bl)
    return out


def pezzi(testo: str, maxc: int = MAX_SOTTOTITOLO) -> list[str]:
    """Una riga di sottotitolo alla volta: a parole, chiudendo dopo virgole e punti se è già lunga."""
    out, cur = [], ""
    for p in testo.split():
        if cur and len(cur) + 1 + len(p) > maxc:
            out.append(cur); cur = p
        else:
            cur = f"{cur} {p}".strip()
        if cur.endswith((",", ":", ";", ".", "!", "?", "»")) and len(cur) > maxc * 0.5:
            out.append(cur); cur = ""
    if cur:
        out.append(cur)
    return out


def _ts(s: float) -> str:
    ms = int(round(s * 1000))
    h, ms = divmod(ms, 3_600_000)
    m, ms = divmod(ms, 60_000)
    se, ms = divmod(ms, 1000)
    return f"{h:02d}:{m:02d}:{se:02d},{ms:03d}"


def srt(tempi: list[dict], spost: float = 0.0) -> str:
    """tempi: [{"testo", "da", "a"}] -> sottotitoli, tempi dei pezzi in proporzione alle lettere."""
    righe, n = [], 1
    for t in tempi:
        da, a = spost + t["da"], spost + t["a"]
        ps = pezzi(t["testo"])
        tot = sum(len(p) for p in ps) or 1
        t0 = da
        for p in ps:
            t1 = t0 + (a - da) * len(p) / tot
            righe.append(f"{n}\n{_ts(t0)} --> {_ts(min(t1 + 0.3, a + 0.6))}\n{p}\n")
            n += 1
            t0 = t1
    return "\n".join(righe)


def parlato(campioni, rate: int, finestra: float = 0.02, soglia_db: float = -32.0,
            pausa_min: float = 0.16, voce_min: float = 0.08) -> list[tuple[float, float]]:
    """Intervalli (da, a) in secondi in cui la voce si sente davvero (virgole e respiri esclusi).

    Volume a finestre di 20 ms; «voce» = sopra `soglia_db` rispetto alla finestra più forte.
    Pause più corte di `pausa_min` non chiudono la bocca; suoni più corti di `voce_min` non la aprono."""
    n = max(1, int(rate * finestra))
    rms = []
    for i in range(0, len(campioni), n):
        pezzo = campioni[i:i + n]
        rms.append((sum(c * c for c in pezzo) / len(pezzo)) ** 0.5 if len(pezzo) else 0.0)
    if not rms or max(rms) <= 0:
        return []
    soglia = max(rms) * 10 ** (soglia_db / 20)
    grezzi, inizio = [], None
    for k, v in enumerate(rms + [0.0]):
        if v >= soglia and inizio is None:
            inizio = k
        elif v < soglia and inizio is not None:
            grezzi.append([inizio * finestra, min(k * finestra, len(campioni) / rate)])
            inizio = None
    uniti = []
    for da, a in grezzi:
        if uniti and da - uniti[-1][1] < pausa_min:
            uniti[-1][1] = a
        else:
            uniti.append([da, a])
    return [(round(da, 3), round(a, 3)) for da, a in uniti if a - da >= voce_min]


def maschera(tempi: list[dict], spost: float, totale: float, fps: int = 24, rampa: int = 3) -> list[int]:
    """Peso della clip «parla» fotogramma per fotogramma (0 = bocca chiusa, 255 = parla).

    Segue gli intervalli di voce vera di ogni blocco (chiave "voce", altrimenti tutto il blocco);
    passa da una clip all'altra in `rampa` fotogrammi; apre un attimo prima della voce."""
    intervalli = sorted((spost + da - 0.04, spost + a)
                        for x in tempi for da, a in (x.get("voce") or [(x["da"], x["a"])]))
    out, w, j = [], 0.0, 0
    passo = 255 / max(1, rampa)
    for k in range(int(round(totale * fps))):
        t = (k + 0.5) / fps
        while j < len(intervalli) and intervalli[j][1] <= t:
            j += 1
        dentro = j < len(intervalli) and intervalli[j][0] <= t
        w = min(255.0, w + passo) if dentro else max(0.0, w - passo)
        out.append(int(round(w)))
    return out


def adesso() -> datetime:
    return datetime.now(timezone.utc)
