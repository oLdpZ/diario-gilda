"""Comandi del Cronista: prepara | pubblica <file.md> [--prova] | chiudi [--ricorrenza TESTO]..."""
from __future__ import annotations

import argparse
import json
import os
import sys
import time
from dataclasses import asdict
from datetime import date, timedelta
from pathlib import Path

from . import ingresso, momenti, pagina, stato
from .discord import Discord, DiscordError
from .stato import Stato

QUI = Path(__file__).resolve().parent.parent  # bot/
CONFIG = QUI / "config.json"
NIENTE = "Niente da raccontare oggi."
TITOLO_GIORNO = "Cronache della Gilda — Giorno {n}, Era della Rinascita"
TITOLO_RECUPERO = "Le Cronache dei primi giorni"
MAX_RICORRENZE_GIORNO = 3
MAX_RICORRENZE = 20
MAX_LUNGHEZZA_RICORRENZA = 150
RITARDO_MAX_GIORNI = 7
MAX_AVVISI_STAMPATI = 30
ERRORI_DATI = (ValueError, TypeError, KeyError, OverflowError, OSError)


class Errore(Exception):
    """Errore da mostrare così com'è (niente traceback)."""


# --- utilità -------------------------------------------------------------------

def _lavoro() -> Path:
    return Path(os.environ.get("CRONISTA_LAVORO") or QUI / "lavoro")


def _config() -> dict:
    try:
        return json.loads(CONFIG.read_text(encoding="utf-8"))
    except (OSError, ValueError) as e:
        raise Errore(f"config non leggibile ({CONFIG}): {e}") from None


def _mostra(p: Path) -> str:
    """Percorso relativo alla cartella corrente se possibile (con /)."""
    try:
        return p.resolve().relative_to(Path.cwd().resolve()).as_posix()
    except ValueError:
        return str(p)


def _chiave(p: Path | str) -> str:
    return os.path.normcase(str(Path(p).resolve()))


def _canali(d: Discord, cfg: dict) -> tuple[str, str]:
    srv = d.server(cfg.get("server"))
    return d.canale(srv["id"], cfg["canale_diario"]), d.canale(srv["id"], cfg["canale_servizio"])


def _scrivi_json(p: Path, dati) -> None:
    p.write_text(json.dumps(dati, ensure_ascii=False, indent=1, default=str), encoding="utf-8")


def _giocatori(sessioni: list[dict]) -> dict[str, str]:
    """giocatore -> "Personaggio, CLASSE Razza, livello N" (dall'ultima sessione)."""
    out: dict[str, str] = {}
    for s in sorted(sessioni, key=lambda s: s["sessione"]["start"]):
        se = s["sessione"]
        parti = [str(se.get("char") or "")]
        cr = " ".join(str(se[k]) for k in ("class", "race") if se.get(k))
        if cr:
            parti.append(cr)
        if se.get("level") is not None:
            parti.append(f"livello {se['level']}")
        out[s["giocatore"]] = ", ".join(p for p in parti if p)
    return dict(sorted(out.items()))


def _pulisci_lavoro(cartella: Path) -> None:
    cartella.mkdir(parents=True, exist_ok=True)
    for pat in ("giorno-*.md", "giorno-*.json", "pagina-*.md", "recupero.md",
                "prepara.json", "prepara-chiuso.json"):
        for f in cartella.glob(pat):
            f.unlink()


# --- prepara -------------------------------------------------------------------

def _per_giornata(sessioni: list[dict], fuso: str, inizio: str):
    """({giorno: [s]} in ordine, avvisi): una sessione che dà errore diventa un avviso."""
    out: dict[str, list[dict]] = {}
    avvisi = []
    for s in sessioni:
        try:
            g = ingresso.giornata(s["sessione"]["start"], fuso, inizio)
        except ERRORI_DATI as e:
            avvisi.append(f"Sessione {s['sessione'].get('id')} di {s.get('giocatore')} ignorata: "
                          f"giornata non calcolabile ({e})")
            continue
        out.setdefault(g, []).append(s)
    return dict(sorted(out.items())), avvisi


def _dividi(sessioni: list[dict], st: Stato, cfg: dict, ora: float):
    """Sessioni divise in: pronte {giorno: [s]}, ritardo, trattenute, vecchie, future, avvisi.

    ritardo/vecchie/future sono liste di (giorno, s); trattenute di s (oggi e domani).
    """
    fuso, inizio = cfg.get("fuso", "Europe/Rome"), cfg.get("inizio_giornata", "06:00")
    oggi = ingresso.giornata(int(ora), fuso, inizio)
    domani = (date.fromisoformat(oggi) + timedelta(days=1)).isoformat()
    pubblicati = set(st.giorni_pubblicati)
    ultimo_pubbl = max(pubblicati, default="")
    pronte: dict[str, list[dict]] = {}
    ritardo, trattenute, future = [], [], []
    per_g, avvisi = _per_giornata(sessioni, fuso, inizio)
    for g, ss in per_g.items():
        if g > domani:
            future.extend((g, s) for s in ss)  # orologio sbagliato: non si aspetta per sempre
        elif g >= oggi:
            trattenute.extend(ss)  # giornata non ancora finita
        elif g in pubblicati or g < ultimo_pubbl:
            ritardo.extend((g, s) for s in ss)
        else:
            pronte[g] = ss
    vecchie = []
    if pronte:
        primo = date.fromisoformat(min(pronte))
        limite = (primo - timedelta(days=RITARDO_MAX_GIORNI)).isoformat()
        vecchie = [(g, s) for g, s in ritardo if g < limite]
        ritardo = [(g, s) for g, s in ritardo if g >= limite]
    return pronte, ritardo, trattenute, vecchie, future, avvisi


def _stampa_avvisi(avvisi: list[str]) -> None:
    print(f"Avvisi: {len(avvisi)}")
    for a in avvisi[:MAX_AVVISI_STAMPATI]:
        print(f"  - {a}")
    if len(avvisi) > MAX_AVVISI_STAMPATI:
        print(f"  … e altri {len(avvisi) - MAX_AVVISI_STAMPATI} avvisi.")


def _note(titolo: str, file_pagina: Path, ricorrenze: list[str]) -> list[str]:
    righe = ["", "## Per la pagina", "",
             f"Titolo: {titolo}", f"File da scrivere: {_mostra(file_pagina)}", "", "Ricorrenze:"]
    righe += [f"- {r}" for r in ricorrenze] if ricorrenze else ["Nessuna."]
    return righe


def _visti(sessioni, ora: float) -> dict[str, dict]:
    return {s["sessione"]["id"]: {"eventi": ingresso.conta_eventi(s), "visto": int(ora)} for s in sessioni}


def _istantanea(st: Stato, giorni: list[str], numero: int, contatori: dict,
                viste: dict[str, dict], cursore: str | None) -> dict:
    """Stato dopo la pubblicazione di una pagina (lo scrive `pubblica`)."""
    nuovo = Stato(**{k: v for k, v in asdict(st).items() if k != "versione"})
    nuovo.giorni_pubblicati = sorted(set(st.giorni_pubblicati) | set(giorni))
    nuovo.numero_giorno = numero
    nuovo.contatori = contatori
    nuovo.sessioni_viste = {**st.sessioni_viste, **viste}
    nuovo.ultimo_messaggio = cursore
    return asdict(nuovo)


def prepara(ora: float) -> int:
    cfg = _config()
    cartella = _lavoro()
    _pulisci_lavoro(cartella)
    d = Discord()
    _, servizio = _canali(d, cfg)
    st = stato.leggi(d, servizio)
    racc = ingresso.raccogli(d, servizio, st)
    pronte, ritardo, trattenute, vecchie, future, avvisi_giorni = _dividi(racc.sessioni, st, cfg, ora)
    avvisi_lettura = racc.avvisi + avvisi_giorni
    if not pronte:
        if avvisi_lettura:
            _stampa_avvisi(avvisi_lettura)
        print(NIENTE)
        return 0

    fuso, inizio = cfg.get("fuso", "Europe/Rome"), cfg.get("inizio_giornata", "06:00")
    giorni = sorted(pronte)
    extra = [s for _, s in vecchie + future]  # viste con la prima pagina, mai raccontate
    avvisi_extra: dict[str, list[str]] = {g: [] for g in giorni}
    for g, s in ritardo:
        pronte[giorni[0]].append(s)
        avvisi_extra[giorni[0]].append(
            f"Sessione di {s['giocatore']} del {g} arrivata in ritardo: inclusa in questa giornata "
            f"(i suoi momenti hanno «in ritardo: {g}»).")
    if vecchie:
        chi = ", ".join(sorted({s["giocatore"] for _, s in vecchie}))
        quando = ", ".join(sorted({g for g, _ in vecchie}))
        avvisi_extra[giorni[0]].append(
            f"{len(vecchie)} sessioni troppo vecchie ignorate (giocatori: {chi}; giornate: {quando}).")
    for g, s in future:
        avvisi_extra[giorni[0]].append(
            f"Sessione di {s['giocatore']} con data nel futuro ({g}): ignorata (orologio del PC sbagliato?).")
    recupero = len(giorni) > 1 and not st.giorni_pubblicati

    # contatori a catena: ogni giornata vede quelli di prima, poi li aggiorna
    contatori = st.contatori
    dopo: dict[str, dict] = {}  # contatori dopo ciascuna giornata
    riassunti: dict[str, str] = {}
    pagine = []
    n = st.numero_giorno
    for i, g in enumerate(giorni):
        ss = sorted(pronte[g], key=lambda s: (s["sessione"]["start"], s["sessione"]["id"]))
        cand = momenti.candidati(ss, contatori)
        contatori = momenti.aggiorna_contatori(contatori, ss)
        dopo[g] = contatori
        for c in cand:
            q = c.get("quando")
            if isinstance(q, int) and not isinstance(q, bool):
                try:
                    gq = ingresso.giornata(q, fuso, inizio)
                except ERRORI_DATI:
                    continue
                if gq < g and isinstance(c.get("fatti"), dict):
                    c["fatti"]["in_ritardo"] = gq
        avv = (avvisi_lettura if i == 0 else []) + avvisi_extra[g]
        gioc = _giocatori(ss)
        md = momenti.riassunto_md(g, cand, avv, gioc, fuso=fuso)
        if not recupero:
            n += 1
            titolo = TITOLO_GIORNO.format(n=n)
            file_pagina = cartella / f"pagina-{g}.md"
            md = md.rstrip("\n") + "\n" + "\n".join(_note(titolo, file_pagina, st.ricorrenze)) + "\n"
            pagine.append({"numero": n, "titolo": titolo, "giorni": [g],
                           "riassunti": [str(cartella / f"giorno-{g}.md")], "pagina": str(file_pagina)})
        riassunti[g] = md
        (cartella / f"giorno-{g}.md").write_text(md, encoding="utf-8")
        _scrivi_json(cartella / f"giorno-{g}.json", {
            "giorno": g, "giocatori": gioc, "candidati": cand, "avvisi": avv, "sessioni": ss})

    if recupero:
        n += 1
        file_pagina = cartella / "pagina-recupero.md"
        righe = [f"# {TITOLO_RECUPERO}", "",
                 f"Pagina di recupero: un solo racconto per {len(giorni)} giornate "
                 f"({', '.join(giorni)}). Conta come Giorno {n}."]
        righe += _note(TITOLO_RECUPERO, file_pagina, st.ricorrenze)
        for g in giorni:
            righe.append("")
            righe += [("#" + r if r.startswith("#") else r) for r in riassunti[g].rstrip("\n").split("\n")]
        (cartella / "recupero.md").write_text("\n".join(righe) + "\n", encoding="utf-8")
        pagine.append({"numero": n, "titolo": TITOLO_RECUPERO, "giorni": giorni,
                       "riassunti": [str(cartella / "recupero.md")], "pagina": str(file_pagina)})

    # cursore finale: le giornate non finite si rileggono domani, prima del loro messaggio
    cursore = racc.ultimo_messaggio
    attesa = [int(racc.messaggi[s["sessione"]["id"]]) for s in trattenute
              if s["sessione"]["id"] in racc.messaggi]
    if attesa:
        blocco = str(min(attesa) - 1)
        cursore = blocco if cursore is None or int(blocco) < int(cursore) else cursore

    # un'istantanea dello stato per pagina: `pubblica` la salva subito dopo ogni pagina
    viste: dict[str, dict] = {}
    fatti: list[str] = []
    for k, p in enumerate(pagine):
        for g in p["giorni"]:
            viste.update(_visti(pronte[g], ora))
        if k == 0:
            viste.update(_visti(extra, ora))
        fatti += p["giorni"]
        ultima = k == len(pagine) - 1
        p["stato"] = _istantanea(st, fatti, p["numero"], dopo[p["giorni"][-1]], viste,
                                 cursore if ultima else st.ultimo_messaggio)
    _scrivi_json(cartella / "prepara.json", {"versione": 2, "creato": int(ora), "pagine": pagine,
                                             "pubblicate": [], "stato": pagine[-1]["stato"]})

    tutti = sorted({s["giocatore"] for g in giorni for s in pronte[g]})
    print(f"Giornate pronte: {', '.join(giorni)}")
    print(f"Giocatori: {', '.join(tutti)}")
    _stampa_avvisi(avvisi_lettura + [a for g in giorni for a in avvisi_extra[g]])
    if trattenute:
        print(f"Sessioni della giornata in corso rimandate a domani: {len(trattenute)}")
    print("Pagine da scrivere (in quest'ordine):")
    for p in pagine:
        print(f"  {p['numero']}. leggi {', '.join(_mostra(Path(r)) for r in p['riassunti'])}"
              f" -> scrivi {_mostra(Path(p['pagina']))} con titolo «{p['titolo']}»")
    return 0


# --- prepara.json --------------------------------------------------------------

def _carica_prep(prep_file: Path) -> dict:
    """prepara.json letto e controllato; Errore chiaro se manca o è rovinato."""
    if not prep_file.exists():
        raise Errore("manca bot/lavoro/prepara.json: esegui prima `prepara`")
    try:
        prep = json.loads(prep_file.read_text(encoding="utf-8"))
        if not isinstance(prep, dict) or not isinstance(prep.get("stato"), dict):
            raise ValueError("manca lo stato")
        pagine = prep["pagine"]
        if not isinstance(pagine, list) or not pagine:
            raise ValueError("nessuna pagina")
        for p in pagine:
            if not (isinstance(p, dict) and isinstance(p.get("pagina"), str)
                    and isinstance(p.get("titolo"), str) and isinstance(p.get("stato"), dict)):
                raise ValueError("pagina non valida")
        if not isinstance(prep.get("pubblicate", []), list):
            raise ValueError("elenco delle pagine pubblicate non valido")
        prep.setdefault("pubblicate", [])
    except (OSError, ValueError, KeyError, TypeError) as e:
        raise Errore(f"bot/lavoro/prepara.json rovinato ({e}): esegui di nuovo `prepara`") from None
    return prep


def _stato_da(dati: dict) -> Stato:
    try:
        return Stato(**{k: v for k, v in dati.items() if k != "versione"})
    except TypeError as e:
        raise Errore(f"stato in attesa non valido in prepara.json ({e}): esegui di nuovo `prepara`") from None


# --- pubblica ------------------------------------------------------------------

def _controlla(p: Path, prova: bool, prep_file: Path):
    """(messaggi, prepara.json, pagina attesa) o Errore: nulla è stato inviato."""
    try:
        testo = p.read_text(encoding="utf-8")
    except OSError as e:
        raise Errore(f"pagina non leggibile: {p} ({e.strerror or e})") from None
    testo = testo.lstrip("\ufeff")
    if not testo.lstrip().startswith("# "):
        raise Errore("la pagina deve iniziare con il titolo: una riga «# Titolo»")
    prep = attesa = None
    if not prova:
        prep = _carica_prep(prep_file)
        per_chiave = {_chiave(x["pagina"]): x for x in prep["pagine"]}
        attesa = per_chiave.get(_chiave(p))
        if attesa is None:
            raise Errore(f"{_mostra(p)} non è una delle pagine attese da `prepara`: "
                         + ", ".join(_mostra(Path(x["pagina"])) for x in prep["pagine"]))
        prima = testo.split("\n", 1)[0].rstrip()
        if prima != f"# {attesa['titolo']}":
            raise Errore(f"la prima riga deve essere esattamente «# {attesa['titolo']}»")
        if _chiave(p) in {_chiave(x) for x in prep["pubblicate"]}:
            raise Errore(f"{_mostra(p)} è già stata pubblicata: non la ripubblico")
        fatte = {_chiave(x) for x in prep["pubblicate"]}
        for x in prep["pagine"]:
            if x is attesa:
                break
            if _chiave(x["pagina"]) not in fatte:
                raise Errore(f"pubblica prima {_mostra(Path(x['pagina']))} (le pagine vanno in ordine)")
    try:
        messaggi = pagina.embed(testo)
    except ValueError as e:
        raise Errore(f"pagina non valida: {e}") from None
    return messaggi, prep, attesa


def pubblica(file: str, prova: bool, ora: float) -> int:
    p = Path(file)
    prep_file = _lavoro() / "prepara.json"
    try:
        messaggi, prep, attesa = _controlla(p, prova, prep_file)
    except Errore as e:
        raise Errore(f"pagina non inviata: {e}") from None
    cfg = _config()
    d = Discord()
    diario, servizio = _canali(d, cfg)
    for m in messaggi:
        d.invia(servizio if prova else diario, embed=m)
    nome = cfg["canale_servizio"] if prova else cfg["canale_diario"]
    print(f"Pubblicata {_mostra(p)} in #{nome}: {len(messaggi)} messaggi.")
    if prova:
        return 0
    prep["pubblicate"].append(str(p.resolve()))
    _scrivi_json(prep_file, prep)
    # lo stato si salva subito: se poi qualcosa fallisce, domani questa pagina non si ripete
    try:
        stato.scrivi(d, servizio, _stato_da(attesa["stato"]), ora=ora)
    except DiscordError as e:
        raise Errore(f"pagina pubblicata ma stato non salvato: {e}") from None
    print(f"Stato salvato: Giorno {attesa['numero']}.")
    return 0


# --- chiudi --------------------------------------------------------------------

def _ricorrenze(nuove: list[str]) -> tuple[list[str], list[str]]:
    """(ricorrenze accettate, messaggi di avviso)."""
    ok, avvisi = [], []
    for r in nuove:
        r = " ".join(r.replace("@", "").split())
        if not r:
            continue
        if len(ok) >= MAX_RICORRENZE_GIORNO:
            avvisi.append(f"Ricorrenza ignorata (massimo {MAX_RICORRENZE_GIORNO} al giorno): {r[:60]}")
            continue
        if len(r) > MAX_LUNGHEZZA_RICORRENZA:
            r = r[:MAX_LUNGHEZZA_RICORRENZA - 1].rstrip() + "…"
        ok.append(r)
    return ok, avvisi


def chiudi(ricorrenze: list[str], ora: float) -> int:
    prep_file = _lavoro() / "prepara.json"
    if not prep_file.exists():
        raise Errore("manca bot/lavoro/prepara.json: esegui prima `prepara` (e `pubblica`), poi `chiudi`")
    prep = _carica_prep(prep_file)
    fatte = {_chiave(x) for x in prep["pubblicate"]}
    mancanti = [x["pagina"] for x in prep["pagine"] if _chiave(x["pagina"]) not in fatte]
    if mancanti:
        raise Errore("pagine non ancora pubblicate: "
                     + ", ".join(_mostra(Path(x)) for x in mancanti) + ". Stato non salvato.")
    nuovo = _stato_da(prep["stato"])
    ok, avvisi = _ricorrenze(ricorrenze)
    nuovo.ricorrenze = (list(nuovo.ricorrenze) + ok)[-MAX_RICORRENZE:]
    for a in avvisi:
        print(a)
    cfg = _config()
    d = Discord()
    _, servizio = _canali(d, cfg)
    stato.scrivi(d, servizio, nuovo, ora=ora)
    prep_file.replace(prep_file.with_name("prepara-chiuso.json"))
    print(f"Stato salvato: Giorno {nuovo.numero_giorno}, giornate {', '.join(prep['pagine'][-1]['giorni'])}"
          f", ricorrenze aggiunte: {len(ok)}.")
    return 0


# --- ingresso ------------------------------------------------------------------

def _parser() -> argparse.ArgumentParser:
    ap = argparse.ArgumentParser(prog="cronista", description="Il Cronista della Gilda")
    sub = ap.add_subparsers(dest="comando", required=True)
    sub.add_parser("prepara", help="scarica i pacchetti nuovi e prepara i riassunti in bot/lavoro/")
    pb = sub.add_parser("pubblica", help="pubblica una pagina nel canale del diario")
    pb.add_argument("file")
    pb.add_argument("--prova", action="store_true", help="pubblica nel canale di servizio")
    ch = sub.add_parser("chiudi", help="salva lo stato su Discord dopo la pubblicazione")
    ch.add_argument("--ricorrenza", action="append", default=[], metavar="TESTO")
    return ap


def main(argv: list[str] | None = None, ora: float | None = None) -> int:
    args = _parser().parse_args(argv)
    ora = time.time() if ora is None else ora
    try:
        if args.comando == "prepara":
            return prepara(ora)
        if args.comando == "pubblica":
            return pubblica(args.file, args.prova, ora)
        return chiudi(args.ricorrenza, ora)
    except (Errore, DiscordError) as e:
        print(f"Errore: {e}", file=sys.stderr)
        return 1
