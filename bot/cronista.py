"""Il Cronista della Gilda. Dalla radice del repository:

    python bot/cronista.py prepara
    python bot/cronista.py pubblica bot/lavoro/pagina-AAAA-MM-GG.md [--prova]
    python bot/cronista.py chiudi [--ricorrenza "TESTO"]...

Vedi bot/ISTRUZIONI.md. La logica vive nel pacchetto bot/cronista/ (cli.py).
"""
import sys
from pathlib import Path

if __name__ == "__main__":
    # la cartella bot/ è già in sys.path: `cronista` è il pacchetto accanto a questo file
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    for flusso in (sys.stdout, sys.stderr):
        try:
            flusso.reconfigure(encoding="utf-8", errors="replace")
        except (AttributeError, ValueError):
            pass
    from cronista.cli import main

    sys.exit(main())
