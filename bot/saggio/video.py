"""Montaggio con ffmpeg: il Cronista che parla / tace (bocca aperta solo quando si sente la voce),
sottotitoli, voce, vento e musica."""
from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

from . import testo as T

RISORSE = Path(__file__).parent / "risorse"
FPS = 24
MW, MH = 16, 9      # la maschera è minuscola: un solo valore per fotogramma, ingrandito da ffmpeg
INTRO = 3.5         # sigla: musica e stanza prima della voce
CODA = 4.0          # la musica risale e chiude
LIMITE = 9_500_000  # Discord: 10 MB per allegato
STILE_SUB = ("FontName=IM FELL English,Fontsize=22,PrimaryColour=&H00E3EEF5,OutlineColour=&H00000000,"
             "BorderStyle=1,Outline=1.6,Shadow=1,MarginV=38,Italic=1")


def comando(totale: float, uscita: str, crf: int = 27, bitrate: int | None = None,
            altezza: int = 720, larghezza_clip: int = 1280, altezza_clip: int = 720) -> list[str]:
    """Le due clip girano in loop per tutto il video; maschera.raw (un valore per fotogramma)
    decide quanto si vede «parla» sopra «silenzio»."""
    args = ["ffmpeg", "-v", "error", "-y",
            "-stream_loop", "-1", "-i", "silenzio.mp4", "-stream_loop", "-1", "-i", "parla.mp4",
            "-f", "rawvideo", "-pix_fmt", "gray", "-video_size", f"{MW}x{MH}", "-framerate", str(FPS),
            "-i", "maschera.raw",
            "-i", "voce.wav", "-stream_loop", "-1", "-i", "ambiente.mp3", "-stream_loop", "-1", "-i", "musica.mp3"]
    f = [f"[0:v]fps={FPS},trim=duration={totale:.3f},setpts=PTS-STARTPTS,format=gbrp[s]",
         f"[1:v]fps={FPS},trim=duration={totale:.3f},setpts=PTS-STARTPTS,format=gbrp[p]",
         f"[2:v]scale={larghezza_clip}:{altezza_clip}:flags=bilinear,format=gbrp[m]",
         "[s][p][m]maskedmerge,format=yuv420p[mix]"]
    scala = "" if altezza == 720 else f"scale=-2:{altezza},"
    f.append(f"[mix]{scala}fade=t=in:st=0:d=0.8,fade=t=out:st={totale - 1.0:.3f}:d=1.0,"
             f"subtitles=cronaca.srt:fontsdir=font:force_style='{STILE_SUB}'[v]")
    na = 3
    f.append(f"[{na}:a]aresample=48000,pan=stereo|c0=c0|c1=c0,adelay={int(INTRO * 1000)}:all=1,apad,"
             f"atrim=0:{totale:.3f},asplit=3[voce][k1][k2]")
    f.append(f"[{na + 1}:a]atrim=0:{totale:.3f},aresample=48000,aformat=channel_layouts=stereo,volume=13dB[amb]")
    f.append("[amb][k1]sidechaincompress=threshold=0.03:ratio=2.5:attack=60:release=700[ambd]")
    f.append(f"[{na + 2}:a]atrim=0:{totale:.3f},aresample=48000,aformat=channel_layouts=stereo,volume=-9dB[mus]")
    f.append("[mus][k2]sidechaincompress=threshold=0.02:ratio=6:attack=150:release=1500[musd]")
    f.append(f"[voce][ambd][musd]amix=inputs=3:normalize=0,afade=t=in:st=0:d=1.5,"
             f"afade=t=out:st={totale - 2.5:.3f}:d=2.5[a]")
    video = ["-c:v", "libx264", "-preset", "medium", "-tune", "animation", "-pix_fmt", "yuv420p"]
    video += ["-crf", str(crf)] if bitrate is None else \
        ["-b:v", str(bitrate), "-maxrate", str(int(bitrate * 1.3)), "-bufsize", str(bitrate * 2)]
    return args + ["-filter_complex", ";".join(f), "-map", "[v]", "-map", "[a]", *video,
                   "-c:a", "aac", "-b:a", "80k", "-ar", "48000", "-movflags", "+faststart",
                   "-t", f"{totale:.3f}", uscita]


def monta(cartella: Path, voce_wav: Path, tempi: list[dict], durata_voce: float) -> Path:
    """Scrive cartella/cronaca.mp4 sotto i 10 MB (se serve abbassa qualità e risoluzione)."""
    for nome, dest in (("parla.mp4", "parla.mp4"), ("silenzio.mp4", "silenzio.mp4"),
                       ("ambiente.mp3", "ambiente.mp3"), ("musica.mp3", "musica.mp3")):
        shutil.copyfile(RISORSE / nome, cartella / dest)
    (cartella / "font").mkdir(exist_ok=True)
    shutil.copyfile(RISORSE / "IMFellEnglish-Italic.ttf", cartella / "font" / "IMFellEnglish-Italic.ttf")
    if voce_wav.resolve() != (cartella / "voce.wav").resolve():
        shutil.copyfile(voce_wav, cartella / "voce.wav")
    totale = INTRO + durata_voce + CODA
    (cartella / "cronaca.srt").write_text(T.srt(tempi, INTRO), encoding="utf-8")
    pesi = T.maschera(tempi, INTRO, totale, FPS)
    (cartella / "maschera.raw").write_bytes(b"".join(bytes([v]) * (MW * MH) for v in pesi))
    uscita = cartella / "cronaca.mp4"
    prove = [dict(crf=27)]
    budget = int((LIMITE * 8 / totale - 80_000) * 0.92)
    prove += [dict(bitrate=max(budget, 150_000)), dict(bitrate=max(budget, 120_000), altezza=540)]
    for p in prove:
        subprocess.run(comando(totale, uscita.name, **p), cwd=cartella, check=True)
        mb = uscita.stat().st_size
        print(f"  video: {totale:.0f} s, {mb / 1e6:.1f} MB ({p})")
        if mb <= LIMITE:
            return uscita
    raise RuntimeError(f"video troppo grande anche a bassa qualità ({mb / 1e6:.1f} MB)")
