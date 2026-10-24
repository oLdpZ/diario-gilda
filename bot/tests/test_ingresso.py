import gzip
import json
import unittest
from datetime import datetime
from pathlib import Path
from zoneinfo import ZoneInfo

from bot.cronista import ingresso, luasv
from bot.cronista.discord import Discord, DiscordError
from bot.cronista.stato import Stato
from bot.tests.finto_discord import FintoDiscord

DATI = Path(__file__).parent / "dati"
GINO = "sessione_Gino_Realm-Gino-1793000000.json.gz"
MARA = "sessione_Mara_Realm-Mara-1793010000.json.gz"
CORROTTO = "sessione_corrotta.json.gz"
LUA = "DiarioGilda.lua"
ROMA = ZoneInfo("Europe/Rome")


def epoch(a, m, g, h, mi):
    return int(datetime(a, m, g, h, mi, tzinfo=ROMA).timestamp())


def gz(doc) -> bytes:
    return gzip.compress(json.dumps(doc).encode(), mtime=0)


def doc_di(nome):
    return json.loads(gzip.decompress((DATI / nome).read_bytes()))


class TestLeggiPacchetto(unittest.TestCase):
    def test_gino(self):
        (s,) = ingresso.leggi_pacchetto(GINO, (DATI / GINO).read_bytes())
        self.assertEqual(s["giocatore"], "Gino")
        self.assertFalse(s["interrotta"])
        self.assertEqual(s["troncata"], 0)
        self.assertEqual(s["sessione"]["id"], "Realm-Gino-1793000000")
        self.assertEqual(s["combattimenti"][0]["enemyCount"], 6)
        self.assertEqual([e["minHp"] for e in s["sessione"]["events"] if e["k"] == "combat"], [12])
        self.assertNotIn("manuale", s)

    def test_mara_wipe_e_kill(self):
        (s,) = ingresso.leggi_pacchetto(MARA, (DATI / MARA).read_bytes())
        enc = [e for e in s["sessione"]["events"] if e["k"] == "encounter"]
        self.assertEqual([e["ok"] for e in enc], [False, False, False, True])
        self.assertEqual(len(s["combattimenti"]), 3)

    def test_corrotto(self):
        with self.assertRaises(ValueError):
            ingresso.leggi_pacchetto(CORROTTO, (DATI / CORROTTO).read_bytes())

    def test_non_gzip_e_json_sbagliato(self):
        for dati in (b"non e gzip", gzip.compress(b"[1,2]"), gzip.compress(b"{not json"),
                     gz({"formato": 2, "giocatore": "A", "sessione": {}}),
                     gz({"formato": 1, "giocatore": "A", "sessione": {"id": "x"}})):
            with self.assertRaises(ValueError):
                ingresso.leggi_pacchetto("a.json.gz", dati)

    def test_troncata_e_interrotta(self):
        doc = doc_di(GINO)
        doc["troncata"] = 7
        doc["interrotta"] = True
        (s,) = ingresso.leggi_pacchetto("a.JSON.GZ", gz(doc))
        self.assertEqual(s["troncata"], 7)
        self.assertTrue(s["interrotta"])
        self.assertEqual(ingresso.conta_eventi(s), 4 + 7)

    def test_lua_manuale(self):
        sess = ingresso.leggi_pacchetto(LUA, (DATI / LUA).read_bytes())
        self.assertEqual(len(sess), 2)
        for s in sess:
            self.assertTrue(s["manuale"])
            self.assertEqual(s["giocatore"], "Gino")
            self.assertEqual(s["combattimenti"], [])
        self.assertFalse(sess[0]["interrotta"])
        self.assertFalse(sess[1]["interrotta"])  # senza stop ma la piu' recente del file
        self.assertEqual(len(sess[0]["sessione"]["events"]), 9)

    def test_start_stop_fuori_intervallo(self):
        for campo, v in (("start", 10**15), ("start", 10**30), ("start", -10**12), ("stop", 10**15), ("stop", -1)):
            doc = doc_di(GINO)
            doc["sessione"][campo] = v
            with self.assertRaises(ValueError, msg=(campo, v)):
                ingresso.leggi_pacchetto("a.json.gz", gz(doc))

    def test_json_ricorsivo_e_gzip_bomba(self):
        for dati in (gzip.compress(b"[" * 100000), gzip.compress(b"0" * (33 * 1024 * 1024))):
            with self.assertRaises(ValueError):
                ingresso.leggi_pacchetto("a.json.gz", dati)

    def test_lua_interrotta_relativa_al_file(self):
        base = 1793000000
        def sess(i, t0, stop=""):
            return ('{ ["id"] = "r-c-%d", ["char"] = "C", ["start"] = %d, %s ["events"] = { { ["t"] = %d, ["k"] = "zone" }, }, },'
                    % (i, t0, stop, t0 + 10))
        lua = "DiarioGildaDB = { sessions = { %s %s %s } }" % (
            sess(1, base), sess(2, base + 20 * 3600), sess(3, base + 100, '["stop"] = %d,' % (base + 200)))
        a, b, c = ingresso.leggi_pacchetto(LUA, lua.encode())
        self.assertTrue(a["interrotta"])    # senza stop, 20 h prima del piu' recente
        self.assertFalse(b["interrotta"])   # e' la piu' recente
        self.assertFalse(c["interrotta"])   # ha stop

    def test_lua_illeggibile_e_tipo_ignoto(self):
        with self.assertRaises(ValueError):
            ingresso.leggi_pacchetto("diariogilda.LUA", b"DiarioGildaDB = { sessions = ")
        with self.assertRaises(ValueError):
            ingresso.leggi_pacchetto(LUA, b"Altro = {}")
        with self.assertRaises(ValueError):
            ingresso.leggi_pacchetto("foto.png", b"")


class TestLuasv(unittest.TestCase):
    def test_tipi_e_vuoto(self):
        r = luasv.carica('﻿A = {\r\n x = 1, y = -2.5, z = "a\\n\\"b\\65", w = true, n = nil, e = {}, -- c\r\n'
                         ' [[lungo]], --[[ blocco ]] 010, }\r\nB = 0x10 C = 99999999999999999999')
        a = r["A"]
        self.assertEqual(a["x"], 1)
        self.assertIsInstance(a["x"], int)
        self.assertEqual(a["y"], -2.5)
        self.assertEqual(a["z"], 'a\n"bA')
        self.assertIs(a["w"], True)
        self.assertNotIn("n", a)
        self.assertEqual(a["e"], [])
        self.assertEqual(a["1"], "lungo")
        self.assertEqual(a["2"], 10)  # base 10: 010 -> 10
        self.assertEqual(r["B"], 16)
        self.assertIsInstance(r["C"], float)

    def test_lista(self):
        self.assertEqual(luasv.carica('A = { "a", "b", { 1, 2 } }')["A"], ["a", "b", [1, 2]])

    def test_profondita(self):
        luasv.carica("A = " + "{" * 200 + "}" * 200)
        with self.assertRaises(ValueError):
            luasv.carica("A = " + "{" * 201 + "}" * 201)

    def test_errori(self):
        for t in ("A = {", 'A = "x', "A 1", "A = foo", "A = [[x", 'A = "\\xZZ"', "A = {1 2"):
            with self.assertRaises(ValueError, msg=t):
                luasv.carica(t)


class TestGiornata(unittest.TestCase):
    def test_soglia(self):
        casi = {(24, 5, 59): "2026-10-23", (24, 6, 0): "2026-10-24",
                (24, 23, 59): "2026-10-24", (26, 0, 30): "2026-10-25"}
        for (g, h, mi), atteso in casi.items():
            self.assertEqual(ingresso.giornata(epoch(2026, 10, g, h, mi), "Europe/Rome", "06:00"), atteso)

    def test_cambio_ora_25_ottobre(self):
        # il 25/10/2026 alle 03:00 CEST si torna alle 02:00 CET
        self.assertEqual(ingresso.giornata(epoch(2026, 10, 25, 5, 59), "Europe/Rome", "06:00"), "2026-10-24")
        self.assertEqual(ingresso.giornata(epoch(2026, 10, 25, 6, 0), "Europe/Rome", "06:00"), "2026-10-25")
        u = int(datetime(2026, 10, 25, 4, 59, tzinfo=ZoneInfo("UTC")).timestamp())  # 05:59 CET
        self.assertEqual(ingresso.giornata(u, "Europe/Rome", "06:00"), "2026-10-24")
        self.assertEqual(ingresso.giornata(u + 60, "Europe/Rome", "06:00"), "2026-10-25")
        for fold in (0, 1):  # 02:30 esiste due volte
            t = int(datetime(2026, 10, 25, 2, 30, tzinfo=ROMA, fold=fold).timestamp())
            self.assertEqual(ingresso.giornata(t, "Europe/Rome", "06:00"), "2026-10-24")

    def test_per_giornata(self):
        def s(t):
            return {"sessione": {"id": str(t), "start": t}}
        a, b, c = s(epoch(2026, 10, 24, 5, 59)), s(epoch(2026, 10, 24, 6, 0)), s(epoch(2026, 10, 24, 22, 0))
        self.assertEqual(ingresso.per_giornata([c, b, a]),
                         {"2026-10-23": [a], "2026-10-24": [c, b]})


class TestRaccogli(unittest.TestCase):
    def setUp(self):
        self.f = FintoDiscord().avvia()
        self.addCleanup(self.f.ferma)
        self.d = Discord(self.f.base, None)

    def invia(self, nome, dati, autore="555", testo=""):
        return self.f.aggiungi_allegato("201", testo, nome, dati, autore)

    def test_pacchetti_stato_e_rumore(self):
        self.invia(GINO, (DATI / GINO).read_bytes())
        self.f.aggiungi("201", "ciao a tutti", autore="555")
        self.invia("stato.json", b"{}", autore="999", testo="Stato del Cronista")
        self.invia("foto.png", b"x")
        self.invia(MARA, (DATI / MARA).read_bytes())
        rotto = self.invia(CORROTTO, (DATI / CORROTTO).read_bytes())
        ultimo = self.f.aggiungi("201", "fine", autore="999")
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertEqual([s["giocatore"] for s in r.sessioni], ["Gino", "Mara"])
        self.assertEqual(len(r.avvisi), 1)
        self.assertIn(CORROTTO, r.avvisi[0])
        self.assertIn(rotto["id"], r.avvisi[0])
        self.assertEqual(r.ultimo_messaggio, ultimo["id"])

    def test_copie_unite_episodi_dalla_copia_che_li_ha(self):
        g = self.invia(GINO, (DATI / GINO).read_bytes())  # 4 eventi, con combattimenti
        self.invia(LUA, (DATI / LUA).read_bytes())  # stessa sessione: piu' eventi, senza episodi
        r = ingresso.raccogli(self.d, "201", Stato())
        s = [x for x in r.sessioni if x["sessione"]["id"] == "Realm-Gino-1793000000"][0]
        self.assertGreater(len(s["sessione"]["events"]), 4)  # eventi dalla vincitrice (.lua)
        self.assertEqual(len(s["combattimenti"]), 1)  # episodi dal pacchetto
        self.assertEqual(s["app"], "0.1.0")
        self.assertNotIn("manuale", s)
        self.assertEqual(r.messaggi["Realm-Gino-1793000000"], g["id"])  # si rileggono entrambe

    def test_solo_messaggi_ignorati_avanza_il_cursore(self):
        self.f.aggiungi("201", "chiacchiere", autore="555")
        m = self.f.aggiungi("201", "mio", autore="999")
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertEqual((r.sessioni, r.avvisi, r.ultimo_messaggio), ([], [], m["id"]))

    def test_nessun_messaggio_conserva_il_cursore(self):
        r = ingresso.raccogli(self.d, "201", Stato(ultimo_messaggio="5"))
        self.assertEqual(r.ultimo_messaggio, "5")

    def test_dopo_il_cursore(self):
        m1 = self.invia(GINO, (DATI / GINO).read_bytes())
        self.invia(MARA, (DATI / MARA).read_bytes())
        r = ingresso.raccogli(self.d, "201", Stato(ultimo_messaggio=m1["id"]))
        self.assertEqual([s["giocatore"] for s in r.sessioni], ["Mara"])

    def test_doppione_nel_lotto_tiene_il_piu_ricco(self):
        doc = doc_di(GINO)
        corto = json.loads(json.dumps(doc))
        corto["sessione"]["events"] = corto["sessione"]["events"][:2]
        self.invia("a.json.gz", gz(doc))
        self.invia("b.json.gz", gz(corto))
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertEqual(len(r.sessioni), 1)
        self.assertEqual(len(r.sessioni[0]["sessione"]["events"]), 4)

    def test_parita_stop_piu_recente(self):
        doc = doc_di(GINO)
        piu = json.loads(json.dumps(doc))
        piu["sessione"]["stop"] += 100
        self.invia("a.json.gz", gz(piu))
        self.invia("b.json.gz", gz(doc))
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertEqual(r.sessioni[0]["sessione"]["stop"], doc["sessione"]["stop"] + 100)

    def test_contro_lo_stato(self):
        self.invia(GINO, (DATI / GINO).read_bytes())
        self.invia(MARA, (DATI / MARA).read_bytes())
        n_mara = len(doc_di(MARA)["sessione"]["events"])
        st = Stato(sessioni_viste={"Realm-Gino-1793000000": {"eventi": 4, "visto": 1},
                                   "Realm-Mara-1793010000": {"eventi": n_mara - 1, "visto": 1}})
        r = ingresso.raccogli(self.d, "201", st)
        self.assertEqual([s["giocatore"] for s in r.sessioni], ["Mara"])  # Gino invariato, Mara cresciuta
        st.sessioni_viste["Realm-Mara-1793010000"]["eventi"] = n_mara + 5  # lo stato ne ha di piu'
        st.sessioni_viste["Realm-Gino-1793000000"]["eventi"] = 9
        self.assertEqual(ingresso.raccogli(self.d, "201", st).sessioni, [])

    def test_sessione_cresciuta_rientra_dal_lua(self):
        self.invia(LUA, (DATI / LUA).read_bytes(), autore="777")
        st = Stato(sessioni_viste={"Realm-Gino-1793000000": {"eventi": 4, "visto": 1}})
        r = ingresso.raccogli(self.d, "201", st)  # il .lua ha 9 eventi: rientra, e c'e' una sessione nuova
        self.assertEqual(sorted(s["sessione"]["id"] for s in r.sessioni),
                         ["Realm-Gino-1793000000", "Realm-Gino-1793086400"])
        self.assertTrue(all(s["manuale"] for s in r.sessioni))

    def test_lua_illeggibile_non_blocca(self):
        self.invia(LUA, b"DiarioGildaDB = {", autore="777")
        self.invia(MARA, (DATI / MARA).read_bytes())
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertEqual([s["giocatore"] for s in r.sessioni], ["Mara"])
        self.assertEqual(len(r.avvisi), 1)
        self.assertIn(LUA, r.avvisi[0])

    def test_download_permanente_avanza(self):
        m = self.invia(GINO, b"x")
        self.f.allegati.clear()  # 404: allegato sparito
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertEqual(r.sessioni, [])
        self.assertIn(m["id"], r.avvisi[0])
        self.assertEqual(r.ultimo_messaggio, m["id"])

    def test_download_transitorio_non_avanza_il_cursore(self):
        m0 = self.invia(MARA, (DATI / MARA).read_bytes())
        m1 = self.invia(GINO, (DATI / GINO).read_bytes())
        m2 = self.invia(MARA, (DATI / MARA).read_bytes())
        vero = self.d.scarica

        def scarica(url):
            if f"/{m1['id']}-" in url:
                raise DiscordError(503, "giu")
            return vero(url)
        self.d.scarica = scarica
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertIn(m1["id"], r.avvisi[0])
        self.assertEqual(r.ultimo_messaggio, str(int(m1["id"]) - 1))
        self.assertGreaterEqual(int(r.ultimo_messaggio), int(m0["id"]))
        self.assertLess(int(r.ultimo_messaggio), int(m2["id"]))
        for status in (0, 429, 500):
            def scarica(url, status=status):
                raise DiscordError(status, "x")
            self.d.scarica = scarica
            r = ingresso.raccogli(self.d, "201", Stato())
            self.assertEqual(r.ultimo_messaggio, str(int(m0["id"]) - 1))

    def test_pacchetto_ostile_non_blocca(self):
        self.invia("a.json.gz", gzip.compress(b"[" * 100000))
        self.invia("b.json.gz", gz({**doc_di(GINO), "sessione": {**doc_di(GINO)["sessione"], "start": 10**30}}))
        self.invia(MARA, (DATI / MARA).read_bytes())
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertEqual([s["giocatore"] for s in r.sessioni], ["Mara"])
        self.assertEqual(len(r.avvisi), 2)
        ingresso.per_giornata(r.sessioni)

    def test_lua_sessione_cattiva_salta_solo_quella(self):
        testo = (DATI / LUA).read_text(encoding="utf-8").replace("[\"start\"] = 1793086400", "[\"start\"] = 10 ^ 40")
        self.assertIn("10 ^ 40", testo)
        testo = testo.replace("10 ^ 40", "1e40")
        self.invia(LUA, testo.encode(), autore="777")
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertEqual([s["sessione"]["id"] for s in r.sessioni], ["Realm-Gino-1793000000"])
        self.assertEqual(len(r.avvisi), 1)

    def test_stato_con_voce_malformata(self):
        self.invia(GINO, (DATI / GINO).read_bytes())
        for voce in (5, None, "x", {"eventi": "4"}, {}):
            st = Stato(sessioni_viste={"Realm-Gino-1793000000": voce})
            self.assertEqual(len(ingresso.raccogli(self.d, "201", st).sessioni), 1)

    def test_primo_avvio_piu_giorni(self):
        self.invia(GINO, (DATI / GINO).read_bytes())
        self.invia(MARA, (DATI / MARA).read_bytes())
        self.invia(LUA, (DATI / LUA).read_bytes(), autore="777")
        r = ingresso.raccogli(self.d, "201", Stato())
        giorni = ingresso.per_giornata(r.sessioni)
        self.assertEqual(list(giorni), sorted(giorni))
        self.assertGreaterEqual(len(giorni), 2)


if __name__ == "__main__":
    unittest.main()
