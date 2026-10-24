import contextlib
import copy
import gzip
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
from datetime import datetime
from pathlib import Path
from zoneinfo import ZoneInfo

from bot.cronista import cli, stato
from bot.cronista.discord import Discord
from bot.tests.dati import genera
from bot.tests.finto_discord import FintoDiscord

DATI = Path(__file__).parent / "dati"
RADICE = Path(__file__).resolve().parents[2]
ROMA = ZoneInfo("Europe/Rome")
SERVER = "1562010080940130304"
DIARIO, SERVIZIO = "200", "201"
PACCHETTI = ["sessione_Gino_Realm-Gino-1793000000.json.gz", "sessione_Mara_Realm-Mara-1793010000.json.gz",
             "sessione_Gino_Realm-Gino-1793009900.json.gz", "DiarioGilda.lua", "sessione_corrotta.json.gz"]


def _ora(testo: str) -> float:
    return datetime.fromisoformat(testo).replace(tzinfo=ROMA).timestamp()


ORA_28 = _ora("2026-10-28 10:07")
ORA_27 = _ora("2026-10-27 10:07")


def _gz(doc) -> bytes:
    return gzip.compress(json.dumps(doc).encode("utf-8"), mtime=0)


def _sposta(doc: dict, secondi: int, nuovo_id: str) -> dict:
    """Copia di un pacchetto spostata nel tempo, con un altro id di sessione."""
    doc = copy.deepcopy(doc)
    se = doc["sessione"]
    se["id"] = nuovo_id
    for k in ("start", "stop"):
        se[k] += secondi
    for e in se["events"]:
        for k in ("t", "from", "to"):
            if k in e:
                e[k] += secondi
    for ep in doc["combattimenti"]:
        ep["from"] += secondi
        ep["to"] += secondi
    return doc


class BaseCli(unittest.TestCase):
    def setUp(self):
        self.f = FintoDiscord().avvia()
        self.addCleanup(self.f.ferma)
        self.f.guilds = [{"id": SERVER, "name": "Orda Florentiae"}]
        self.f.canali = {SERVER: [{"id": DIARIO, "name": "diario-di-gilda", "type": 0},
                                  {"id": SERVIZIO, "name": "diario-servizio", "type": 0}]}
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.lavoro = Path(self.tmp.name) / "lavoro"
        env = {"CRONISTA_API": self.f.base, "CRONISTA_LAVORO": str(self.lavoro)}
        vecchio = {k: os.environ.get(k) for k in (*env, "DISCORD_BOT_TOKEN")}

        def ripristina():
            for k, v in vecchio.items():
                if v is None:
                    os.environ.pop(k, None)
                else:
                    os.environ[k] = v
        self.addCleanup(ripristina)
        os.environ.pop("DISCORD_BOT_TOKEN", None)
        os.environ.update(env)

    def carica(self, nomi=PACCHETTI):
        for n in nomi:
            self.f.aggiungi_allegato(SERVIZIO, "", n, (DATI / n).read_bytes(), autore="555")

    def esegui(self, *argv, ora=ORA_28):
        out, err = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            codice = cli.main(list(argv), ora=ora)
        return codice, out.getvalue(), err.getvalue()

    def preparato(self) -> dict:
        return json.loads((self.lavoro / "prepara.json").read_text(encoding="utf-8"))

    def scrivi_pagina(self, nome: str, titolo: str) -> Path:
        p = self.lavoro / nome
        p.write_text(f"# {titolo}\n\nNarrano le genti di Westfall che Gino gettò la lenza.\n",
                     encoding="utf-8")
        return p

    def post(self, canale: str) -> list[dict]:
        return [r for r in self.f.richieste
                if r["metodo"] == "POST" and r["percorso"] == f"/channels/{canale}/messages"]

    def stato_finale(self):
        return stato.leggi(Discord(self.f.base, None), SERVIZIO)


class TestFlusso(BaseCli):
    def test_primo_avvio_recupero_pubblica_chiudi(self):
        self.carica()
        codice, out, err = self.esegui("prepara")
        self.assertEqual(codice, 0, err)
        for n in ("giorno-2026-10-26.md", "giorno-2026-10-26.json", "giorno-2026-10-27.md",
                  "giorno-2026-10-27.json", "recupero.md", "prepara.json"):
            self.assertTrue((self.lavoro / n).exists(), n)
        self.assertIn("2026-10-26", out)
        self.assertIn("2026-10-27", out)
        self.assertIn("pagina-recupero.md", out)
        self.assertIn("Le Cronache dei primi giorni", out)
        self.assertIn("Avvisi: 1", out)
        self.assertEqual(self.post(DIARIO) + self.post(SERVIZIO), [])  # prepara non scrive
        rec = (self.lavoro / "recupero.md").read_text(encoding="utf-8")
        self.assertTrue(rec.startswith("# Le Cronache dei primi giorni"))
        self.assertIn("Edwin VanCleef", rec)
        g26 = (self.lavoro / "giorno-2026-10-26.md").read_text(encoding="utf-8")
        self.assertIn("Gino", g26)
        self.assertIn("Mara", g26)
        self.assertIn("sessione_corrotta.json.gz", g26)  # avviso
        js = json.loads((self.lavoro / "giorno-2026-10-26.json").read_text(encoding="utf-8"))
        self.assertTrue(js["candidati"])

        prep = self.preparato()
        self.assertEqual(len(prep["pagine"]), 1)
        self.assertEqual(prep["pagine"][0]["numero"], 1)
        self.assertEqual(prep["pagine"][0]["giorni"], ["2026-10-26", "2026-10-27"])
        st = prep["stato"]
        self.assertEqual(st["giorni_pubblicati"], ["2026-10-26", "2026-10-27"])
        self.assertEqual(st["numero_giorno"], 1)
        self.assertEqual(set(st["sessioni_viste"]), {"Realm-Gino-1793000000", "Realm-Gino-1793009900",
                                                     "Realm-Mara-1793010000", "Realm-Gino-1793086400"})
        self.assertEqual(st["sessioni_viste"]["Realm-Gino-1793086400"]["eventi"], 3)
        self.assertTrue(st["contatori"])

        pag = self.scrivi_pagina("pagina-recupero.md", "Le Cronache dei primi giorni")
        codice, out, err = self.esegui("pubblica", str(pag))
        self.assertEqual(codice, 0, err)
        posts = self.post(DIARIO)
        self.assertEqual(len(posts), 1)
        self.assertEqual(posts[0]["payload"]["allowed_mentions"], {"parse": []})
        self.assertEqual(posts[0]["payload"]["embeds"][0]["title"], "Le Cronache dei primi giorni")

        codice, out, err = self.esegui("chiudi", "--ricorrenza", "Gino e i Murloc")
        self.assertEqual(codice, 0, err)
        s = self.stato_finale()
        self.assertEqual(s.numero_giorno, 1)
        self.assertEqual(s.giorni_pubblicati, ["2026-10-26", "2026-10-27"])
        self.assertEqual(s.ricorrenze, ["Gino e i Murloc"])
        self.assertEqual(len(s.sessioni_viste), 4)
        self.assertIsNotNone(s.ultimo_messaggio)

        codice, out, err = self.esegui("prepara")
        self.assertEqual(codice, 0, err)
        self.assertEqual(out.strip(), "Niente da raccontare oggi.")
        self.assertFalse((self.lavoro / "prepara.json").exists())

    def test_giornata_in_corso_trattenuta(self):
        self.carica()
        codice, out, err = self.esegui("prepara", ora=ORA_27)
        self.assertEqual(codice, 0, err)
        self.assertTrue((self.lavoro / "giorno-2026-10-26.md").exists())
        self.assertFalse((self.lavoro / "giorno-2026-10-27.md").exists())
        self.assertFalse((self.lavoro / "recupero.md").exists())
        prep = self.preparato()
        self.assertEqual(prep["pagine"][0]["titolo"], "Cronache della Gilda — Giorno 1, Era della Rinascita")
        self.assertNotIn("Realm-Gino-1793086400", prep["stato"]["sessioni_viste"])
        lua = [m for m in self.f.messaggi[SERVIZIO] if m["attachments"][0]["filename"] == "DiarioGilda.lua"][0]
        self.assertLess(int(prep["stato"]["ultimo_messaggio"]), int(lua["id"]))
        self.esegui("pubblica", str(self.scrivi_pagina("pagina-2026-10-26.md", prep["pagine"][0]["titolo"])),
                    ora=ORA_27)
        self.assertEqual(self.esegui("chiudi", ora=ORA_27)[0], 0)

        codice, out, err = self.esegui("prepara")
        self.assertEqual(codice, 0, err)
        prep = self.preparato()
        self.assertEqual([p["giorni"] for p in prep["pagine"]], [["2026-10-27"]])
        self.assertEqual(prep["pagine"][0]["numero"], 2)
        self.assertIn("Giorno 2", prep["pagine"][0]["titolo"])
        self.assertIn("pagina-2026-10-27.md", out)
        js = json.loads((self.lavoro / "giorno-2026-10-27.json").read_text(encoding="utf-8"))
        self.assertEqual([s["sessione"]["id"] for s in js["sessioni"]], ["Realm-Gino-1793086400"])
        self.assertEqual(prep["stato"]["giorni_pubblicati"], ["2026-10-26", "2026-10-27"])

    def test_contatori_a_catena_tra_giorni(self):
        # due giornate gia' pubblicate in passato: niente recupero, una pagina per giorno
        stato.scrivi(Discord(self.f.base, None), SERVIZIO,
                     stato.Stato(giorni_pubblicati=["2026-10-20"], numero_giorno=5), ora=ORA_28)
        m = genera.mara()
        self.f.aggiungi_allegato(SERVIZIO, "", "a.json.gz", _gz(m), autore="555")
        self.f.aggiungi_allegato(SERVIZIO, "", "b.json.gz", _gz(_sposta(m, 86400, "Realm-Mara-2")), autore="555")
        codice, out, err = self.esegui("prepara")
        self.assertEqual(codice, 0, err)
        prep = self.preparato()
        self.assertFalse((self.lavoro / "recupero.md").exists())
        self.assertEqual([(p["numero"], p["giorni"]) for p in prep["pagine"]],
                         [(6, ["2026-10-26"]), (7, ["2026-10-27"])])
        self.assertEqual(prep["stato"]["numero_giorno"], 7)

        def boss(giorno):
            js = json.loads((self.lavoro / f"giorno-{giorno}.json").read_text(encoding="utf-8"))
            return [c for c in js["candidati"] if c["tipo"] == "boss_ucciso"][0]
        self.assertNotIn("tentativi_totali", boss("2026-10-26")["fatti"])
        self.assertEqual(boss("2026-10-27")["fatti"]["tentativi_totali"], 8)

    def test_sessione_in_ritardo_nella_pagina_dopo(self):
        stato.scrivi(Discord(self.f.base, None), SERVIZIO,
                     stato.Stato(giorni_pubblicati=["2026-10-26"], numero_giorno=1), ora=ORA_28)
        m = genera.mara()
        self.f.aggiungi_allegato(SERVIZIO, "", "a.json.gz", _gz(m), autore="555")
        self.f.aggiungi_allegato(SERVIZIO, "", "b.json.gz", _gz(_sposta(m, 86400, "Realm-Mara-2")), autore="555")
        codice, out, err = self.esegui("prepara")
        self.assertEqual(codice, 0, err)
        prep = self.preparato()
        self.assertEqual([p["giorni"] for p in prep["pagine"]], [["2026-10-27"]])
        md = (self.lavoro / "giorno-2026-10-27.md").read_text(encoding="utf-8")
        self.assertIn("in ritardo", md)
        self.assertEqual(set(prep["stato"]["sessioni_viste"]), {"Realm-Mara-1793010000", "Realm-Mara-2"})

    def test_solo_ritardi_niente_pagina(self):
        stato.scrivi(Discord(self.f.base, None), SERVIZIO,
                     stato.Stato(giorni_pubblicati=["2026-10-26"], numero_giorno=1), ora=ORA_28)
        self.f.aggiungi_allegato(SERVIZIO, "", "a.json.gz", _gz(genera.mara()), autore="555")
        codice, out, _ = self.esegui("prepara")
        self.assertEqual((codice, out.strip()), (0, "Niente da raccontare oggi."))


class TestComandi(BaseCli):
    def test_chiudi_senza_prepara(self):
        codice, out, err = self.esegui("chiudi")
        self.assertNotEqual(codice, 0)
        self.assertIn("prepara", err)
        self.assertEqual(self.post(SERVIZIO), [])

    def test_chiudi_rifiuta_pagine_non_pubblicate(self):
        self.carica(PACCHETTI[:2])
        self.assertEqual(self.esegui("prepara")[0], 0)
        codice, _, err = self.esegui("chiudi")
        self.assertNotEqual(codice, 0)
        self.assertIn("pagina-2026-10-26.md", err)
        self.assertEqual(self.post(SERVIZIO), [])

    def test_pubblica_prova_va_nel_servizio_e_non_conta(self):
        self.carica(PACCHETTI[:2])
        self.esegui("prepara")
        titolo = self.preparato()["pagine"][0]["titolo"]
        pag = self.scrivi_pagina("pagina-2026-10-26.md", titolo)
        codice, _, err = self.esegui("pubblica", str(pag), "--prova")
        self.assertEqual(codice, 0, err)
        self.assertEqual(self.post(DIARIO), [])
        self.assertEqual(len(self.post(SERVIZIO)), 1)
        self.assertNotEqual(self.esegui("chiudi")[0], 0)
        self.assertEqual(self.esegui("pubblica", str(pag))[0], 0)
        self.assertNotEqual(self.esegui("pubblica", str(pag))[0], 0)  # mai due volte
        self.assertEqual(len(self.post(DIARIO)), 1)
        self.assertEqual(self.esegui("chiudi")[0], 0)
        self.assertNotEqual(self.esegui("chiudi")[0], 0)  # gia' chiuso

    def test_pubblica_rifiuta_pagina_senza_titolo(self):
        p = self.lavoro / "x.md"
        p.parent.mkdir(parents=True)
        p.write_text("testo senza titolo", encoding="utf-8")
        codice, _, err = self.esegui("pubblica", str(p))
        self.assertNotEqual(codice, 0)
        self.assertIn("titolo", err)
        self.assertEqual(self.post(DIARIO), [])

    def test_ricorrenze_al_massimo_tre_e_brevi(self):
        self.carica(PACCHETTI[:2])
        self.esegui("prepara")
        self.esegui("pubblica", str(self.scrivi_pagina("pagina-2026-10-26.md", "T")))
        codice, out, err = self.esegui("chiudi", "--ricorrenza", "a", "--ricorrenza", "@b",
                                       "--ricorrenza", "c" * 500, "--ricorrenza", "d")
        self.assertEqual(codice, 0, err)
        r = self.stato_finale().ricorrenze
        self.assertEqual(len(r), 3)
        self.assertEqual(r[0], "a")
        self.assertNotIn("@", r[1])
        self.assertLessEqual(len(r[2]), 150)
        self.assertIn("ignorat", out + err)

    def test_errore_discord_chiaro_senza_token(self):
        os.environ["DISCORD_BOT_TOKEN"] = "segreto-123"
        self.f.forza_status = 403
        codice, out, err = self.esegui("prepara")
        self.assertEqual(codice, 1)
        self.assertIn("403", err)
        self.assertNotIn("segreto-123", out + err)
        self.assertNotIn("Traceback", err)


class TestScript(BaseCli):
    def test_script_dalla_radice(self):
        env = dict(os.environ)
        r = subprocess.run([sys.executable, "bot/cronista.py", "prepara"], cwd=RADICE, env=env,
                           capture_output=True, text=True, encoding="utf-8", timeout=60)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout.strip(), "Niente da raccontare oggi.")


if __name__ == "__main__":
    unittest.main()
