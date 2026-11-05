"""La Voce del Saggio: per ogni pagina nuova del Cronista, un video narrato in risposta alla pagina.

    python -m bot.saggio                 # pagine delle ultime 20 ore ancora senza video -> #diario-di-gilda
    python -m bot.saggio --prova         # l'ultima pagina, video in #diario-servizio (per provare)
    python -m bot.saggio --file P.md --titolo "..." --uscita DIR   # solo video, senza Discord

Servono GEMINI_API_KEY e (tranne con --file) DISCORD_BOT_TOKEN. Le chiavi non vengono mai stampate.
La pagina scritta non dipende da questo: se qualcosa va storto, manca solo il video."""
from __future__ import annotations

import argparse
import json
import os
import tempfile
from pathlib import Path

from . import testo as T
from . import video, voce

RADICE = Path(__file__).resolve().parents[1]
MESSAGGIO = "🔊 **Il Cronista Goblin** racconta: *{titolo}*"


def fai_video(titolo: str, testo: str, cartella: Path, chiave: str) -> Path:
    bl = T.blocchi(titolo, testo)
    print(f"  {sum(len(p) for p in bl)} blocchi da leggere in {len(bl)} paragrafi")
    wav, tempi, dur = voce.leggi(bl, cartella, chiave)
    return video.monta(cartella, wav, tempi, dur)


def _chiave() -> str:
    k = os.environ.get("GEMINI_API_KEY", "").strip()
    if not k:
        raise SystemExit("manca GEMINI_API_KEY")
    return k


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(prog="python -m bot.saggio")
    ap.add_argument("--prova", action="store_true", help="ultima pagina, video nel canale di servizio")
    ap.add_argument("--file", help="pagina .md da leggere (niente Discord)")
    ap.add_argument("--titolo", default="Cronache della Gilda")
    ap.add_argument("--uscita", default=".", help="cartella dove lasciare il video con --file")
    a = ap.parse_args(argv)
    chiave = _chiave()

    if a.file:
        testo = Path(a.file).read_text(encoding="utf-8")
        righe = testo.splitlines()
        titolo = a.titolo
        if righe and righe[0].startswith("# "):
            titolo, testo = righe[0][2:].strip(), "\n".join(righe[1:])
        with tempfile.TemporaryDirectory() as tmp:
            v = fai_video(titolo, testo, Path(tmp), chiave)
            dest = Path(a.uscita) / "cronaca.mp4"
            dest.write_bytes(v.read_bytes())
        print(f"Video scritto: {dest}")
        return 0

    from ..cronista.discord import Discord, DiscordError

    if os.environ.get("DISCORD_BOT_TOKEN"):  # un a capo finale nel secret romperebbe l'header
        os.environ["DISCORD_BOT_TOKEN"] = os.environ["DISCORD_BOT_TOKEN"].strip()

    cfg = json.loads((RADICE / "config.json").read_text(encoding="utf-8"))
    d = Discord()
    try:
        srv = d.server(cfg.get("server"))
        diario = d.canale(srv["id"], cfg["canale_diario"])
        servizio = d.canale(srv["id"], cfg["canale_servizio"])
        io = d.io()
        messaggi = d.ultimi(diario, 100)
    except DiscordError as e:
        print(f"Discord non risponde: {e}")
        return 1
    if a.prova:
        tutte = T.pagine(messaggi, io)
        lista = tutte[-1:]
    else:
        lista = T.da_narrare(messaggi, io, T.adesso())
    if not lista:
        print("Nessuna pagina nuova da raccontare.")
        return 0
    errori = 0
    for p in lista:
        print(f"Pagina {p['id']}: {p['titolo']}")
        try:
            with tempfile.TemporaryDirectory() as tmp:
                v = fai_video(p["titolo"], p["testo"], Path(tmp), chiave)
                d.invia(servizio if a.prova else diario, MESSAGGIO.format(titolo=p["titolo"]),
                        file=(f"{T.PREFISSO_FILE}{p['id']}.mp4", v.read_bytes()),
                        risposta=None if a.prova else p["id"])
            print("  pubblicato" + (" in #" + cfg["canale_servizio"] if a.prova else ""))
        except (voce.ErroreVoce, DiscordError, RuntimeError, OSError) as e:
            errori += 1
            print(f"  NON pubblicato: {e}")
        except Exception as e:  # ffmpeg (CalledProcessError) e imprevisti: la prossima pagina ci prova lo stesso
            errori += 1
            print(f"  NON pubblicato: {type(e).__name__}: {e}")
    return 1 if errori else 0
