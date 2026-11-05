"""La voce del Saggio: Gemini 3.8 Flash TTS, un blocco alla volta, poi un po' più svelta (atempo)."""
from __future__ import annotations

import base64
import json
import subprocess
import time
import urllib.error
import urllib.request
import wave
from pathlib import Path

API = "https://generativelanguage.googleapis.com/v1beta/interactions"
MODELLO = "gemini-3.8-flash-tts"
VOCE = "voice_l7tji7qvvm6z"   # goblin roco e furbo (voce personalizzata del progetto di doppiaggio)
STILE = ("Narrate like a sly, excited old goblin chronicler telling the guild's latest deeds by candlelight: "
         "lively and theatrical but always clear, savouring the juicy parts, a little mischievous. "
         "The text is Italian: speak it as a native Italian speaker from Italy, standard Italian pronunciation and "
         "intonation, no English or foreign accent; only proper names from the game stay as written.")
VELOCITA = 1.05
PAUSA_FRASE, PAUSA_PARAGRAFO = 0.55, 1.1
TENTATIVI = 3


class ErroreVoce(Exception):
    pass


def tts(testo: str, chiave: str) -> bytes:
    corpo = json.dumps({
        "model": MODELLO,
        "input": [{"type": "user_input", "content": [{"type": "text", "text": testo,
                   "annotations": [{"type": "speech_metadata", "style": STILE}]}]}],
        "response_format": {"type": "audio"},
        "generation_config": {"speech_config": [{"voice": VOCE}]},
    }).encode()
    ultimo = ""
    for n in range(1, TENTATIVI + 1):
        req = urllib.request.Request(API, data=corpo, headers={
            "x-goog-api-key": chiave, "Content-Type": "application/json", "Api-Revision": "2026-05-20"})
        try:
            with urllib.request.urlopen(req, timeout=180) as r:
                ris = json.loads(r.read())
        except urllib.error.HTTPError as e:
            ultimo = f"HTTP {e.code}: {e.read()[:300].decode('utf-8', 'replace')}"
            if e.code in (429, 500, 502, 503, 504) and n < TENTATIVI:
                time.sleep(10 * n)
                continue
            raise ErroreVoce(ultimo) from None
        except OSError as e:
            ultimo = str(e)
            if n < TENTATIVI:
                time.sleep(10 * n)
                continue
            raise ErroreVoce(ultimo) from None
        dati = None
        for passo in ris.get("steps", []):
            for c in passo.get("content", []) or []:
                if isinstance(c, dict) and c.get("type") == "audio" and c.get("data"):
                    dati = c["data"]
        if dati:
            return base64.b64decode(dati)
        ultimo = "risposta senza audio"
    raise ErroreVoce(ultimo)


def leggi(blocchi: list[list[str]], cartella: Path, chiave: str, parla=tts) -> tuple[Path, list[dict], float]:
    """Legge tutti i blocchi; restituisce (voce.wav, tempi [{"testo","da","a"}], durata totale)."""
    cartella.mkdir(parents=True, exist_ok=True)
    uscita = cartella / "voce.wav"
    tempi, t, n, param = [], 0.0, 0, None
    with wave.open(str(uscita), "wb") as w:
        for pi, par in enumerate(blocchi):
            for bi, testo in enumerate(par):
                grezzo = cartella / f"b{n:03d}.wav"
                veloce = cartella / f"v{n:03d}.wav"
                grezzo.write_bytes(parla(testo, chiave))
                subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", str(grezzo), "-af", f"atempo={VELOCITA}",
                                "-ar", "24000", "-ac", "1", "-c:a", "pcm_s16le", str(veloce)], check=True)
                with wave.open(str(veloce)) as r:
                    if param is None:
                        param = r.getparams()
                        w.setparams(param)
                    w.writeframes(r.readframes(r.getnframes()))
                    dur = r.getnframes() / r.getframerate()
                tempi.append({"testo": testo, "da": round(t, 3), "a": round(t + dur, 3)})
                t += dur
                pausa = PAUSA_PARAGRAFO if bi == len(par) - 1 else PAUSA_FRASE
                w.writeframes(b"\x00" * int(pausa * param.framerate) * param.sampwidth * param.nchannels)
                t += pausa
                n += 1
                print(f"  voce {n:2d}: {dur:5.1f} s")
    return uscita, tempi, t
