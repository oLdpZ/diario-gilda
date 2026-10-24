"""Pagina di diario (Markdown) -> messaggi di embed nei limiti di Discord."""
from __future__ import annotations

COLORE = 0xC8A15A
PIEDE = "Il Cronista della Gilda"
MAX_TITOLO = 256
MAX_DESCRIZIONE = 4096
MAX_EMBED = 10
MAX_MESSAGGIO = 6000
_ZWSP = "​"


def _senza_menzioni(testo: str) -> str:
    # niente @everyone/@nome cliccabili, anche se allowed_mentions già blocca i ping
    return testo.replace("@", "@" + _ZWSP)


def _taglia_parole(testo: str, limite: int) -> list[str]:
    """Divide un testo lungo a parole; una parola più lunga del limite si taglia."""
    pezzi, cur = [], ""
    for parola in testo.split():
        while len(parola) > limite:
            if cur:
                pezzi.append(cur)
                cur = ""
            pezzi.append(parola[:limite])
            parola = parola[limite:]
        if not cur:
            cur = parola
        elif len(cur) + 1 + len(parola) <= limite:
            cur += " " + parola
        else:
            pezzi.append(cur)
            cur = parola
    if cur:
        pezzi.append(cur)
    return pezzi


def _blocchi(corpo: str, limite: int) -> list[str]:
    """Paragrafi uniti fino al limite; i paragrafi troppo lunghi divisi a parole."""
    paragrafi = []
    for p in corpo.split("\n\n"):
        p = p.strip()
        if not p:
            continue
        paragrafi.extend([p] if len(p) <= limite else _taglia_parole(p, limite))
    out, cur = [], ""
    for p in paragrafi:
        if not cur:
            cur = p
        elif len(cur) + 2 + len(p) <= limite:
            cur += "\n\n" + p
        else:
            out.append(cur)
            cur = p
    if cur:
        out.append(cur)
    return out


def _titolo(riga: str) -> str:
    t = riga.lstrip("#").strip()
    if len(t) > MAX_TITOLO:
        t = t[:MAX_TITOLO - 1]
        t = (t.rsplit(" ", 1)[0] if " " in t else t).rstrip() + "…"
    return t


def embed(testo_md: str) -> list[list[dict]]:
    """Messaggi (liste di embed): `# Titolo` in testa diventa il titolo del primo embed."""
    testo = _senza_menzioni(testo_md.replace("\r\n", "\n")).strip()
    titolo = None
    if testo.startswith("# "):
        prima, _, testo = testo.partition("\n")
        titolo = _titolo(prima) or None
    # spazio per titolo e piede nel primo messaggio
    riserva = len(titolo or "") + len(PIEDE)
    limite = min(MAX_DESCRIZIONE, MAX_MESSAGGIO - riserva)
    blocchi = _blocchi(testo.strip(), limite)
    if not blocchi and not titolo:
        raise ValueError("pagina vuota")
    embeds: list[dict] = []
    for i, b in enumerate(blocchi or [""]):
        e: dict = {"color": COLORE}
        if b:
            e["description"] = b
        if i == 0:
            if titolo:
                e["title"] = titolo
            e["footer"] = {"text": PIEDE}
        embeds.append(e)
    messaggi: list[list[dict]] = []
    cur: list[dict] = []
    tot = 0
    for e in embeds:
        n = len(e.get("title", "")) + len(e.get("description", "")) + len(e.get("footer", {}).get("text", ""))
        if cur and (len(cur) >= MAX_EMBED or tot + n > MAX_MESSAGGIO):
            messaggi.append(cur)
            cur, tot = [], 0
        cur.append(e)
        tot += n
    messaggi.append(cur)
    return messaggi
