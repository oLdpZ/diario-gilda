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

    def test_eventi_con_orario_assurdo_scartati(self):
        doc = doc_di(GINO)
        ev = doc["sessione"]["events"]
        n = len(ev)
        ev += [{"t": 10**15, "k": "level", "level": 60}, {"t": 0, "k": "zone", "z": "X"},
               {"t": "x", "k": "zone"}, {"t": True, "k": "zone"}, {"k": "zone"}]
        (s,) = ingresso.leggi_pacchetto("a.json.gz", gz(doc))
        self.assertEqual(len(s["sessione"]["events"]), n)
        # ai margini: un giorno prima dello start e due dopo lo stop restano
        doc = doc_di(GINO)
        se = doc["sessione"]
        se["events"] += [{"t": se["start"] - 86400, "k": "zone"}, {"t": se["stop"] + 2 * 86400, "k": "zone"}]
        (s,) = ingresso.leggi_pacchetto("a.json.gz", gz(doc))
        self.assertEqual(len(s["sessione"]["events"]), n + 2)

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

    def test_lua_solo_personaggio_piu_giocato(self):
        base = 1793000000
        def sess(i, char, t0, n, realm="R"):
            ev = " ".join('{ ["t"] = %d, ["k"] = "zone" },' % (t0 + 10 + j) for j in range(n))
            return ('{ ["id"] = "%s-%s-%d", ["char"] = "%s", ["realm"] = "%s", ["start"] = %d, ["stop"] = %d, '
                    '["events"] = { %s }, },' % (realm, char, i, char, realm, t0, t0 + 500, ev))
        lua = "DiarioGildaDB = { sessions = { %s %s %s %s } }" % (
            sess(1, "Gino", base, 3), sess(2, "Grizzo", base + 1000, 4),
            sess(3, "gino", base + 2000, 2), sess(4, "Gino", base + 3000, 1, realm="Altro"))
        sessioni, avvisi = ingresso._filtra_manuale(ingresso._leggi(LUA, lua.encode())[0], set())
        # Gino/R ha 5 eventi (maiuscole ignorate), Grizzo 4, Gino/Altro 1
        self.assertEqual([s["sessione"]["id"] for s in sessioni], ["R-Gino-1", "R-gino-3"])
        self.assertEqual(len(avvisi), 1)
        self.assertIn("2 sessioni di altri personaggi ignorate", avvisi[0])
        self.assertIn("Grizzo", avvisi[0])
        self.assertIn("Gino", avvisi[0])
        # a parità di eventi vince il personaggio giocato più di recente
        lua = "DiarioGildaDB = { sessions = { %s %s } }" % (sess(1, "Gino", base, 2), sess(2, "Grizzo", base + 1000, 2))
        sessioni, _ = ingresso._filtra_manuale(ingresso._leggi(LUA, lua.encode())[0], set())
        self.assertEqual([s["giocatore"] for s in sessioni], ["Grizzo"])
        # personaggio principale già noto: vince lui anche con meno eventi
        lua = "DiarioGildaDB = { sessions = { %s %s } }" % (sess(1, "Gino", base, 1), sess(2, "Grizzo", base + 1000, 9))
        sessioni, avvisi = ingresso._filtra_manuale(ingresso._leggi(LUA, lua.encode())[0], {"gino|r"})
        self.assertEqual([s["giocatore"] for s in sessioni], ["Gino"])
        self.assertIn("solo i personaggi principali", avvisi[0])
        # un solo personaggio: nessun avviso
        sessioni, avvisi = ingresso._filtra_manuale(ingresso._leggi(LUA, (DATI / LUA).read_bytes())[0], set())
        self.assertEqual((len(sessioni), avvisi), (2, []))

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

    def test_nome_del_giocatore_dalla_copia_del_programma(self):
        for comb in (True, False):
            self.f.messaggi.clear()
            doc = doc_di(GINO)
            doc["giocatore"] = "Ginetto"
            if not comb:
                doc["combattimenti"] = []
            self.invia("a.json.gz", gz(doc))
            self.invia(LUA, (DATI / LUA).read_bytes(), autore="777")  # vince (piu' eventi), nome "Gino"
            r = ingresso.raccogli(self.d, "201", Stato())
            s = [x for x in r.sessioni if x["sessione"]["id"] == "Realm-Gino-1793000000"][0]
            self.assertGreater(len(s["sessione"]["events"]), 4)
            self.assertEqual(s["giocatore"], "Ginetto", comb)

    def test_sessione_ripresa_solo_eventi_nuovi(self):
        doc = doc_di(GINO)
        ev = sorted(doc["sessione"]["events"], key=lambda e: e["t"])
        fino = ev[1]["t"]
        self.invia("a.json.gz", gz(doc))
        st = Stato(sessioni_viste={"Realm-Gino-1793000000": {"eventi": 2, "visto": 1, "fino": fino}})
        (s,) = ingresso.raccogli(self.d, "201", st).sessioni
        self.assertTrue(all(e["t"] > fino for e in s["sessione"]["events"]))
        self.assertEqual(len(s["sessione"]["events"]), len([e for e in ev if e["t"] > fino]))
        self.assertTrue(all(ep["to"] > fino for ep in s["combattimenti"]))
        self.assertEqual(ingresso.conta_eventi(s), 4)  # il conto resta quello intero
        self.assertEqual(ingresso.fino(s), max(e["t"] for e in ev))
        # senza "fino" (stato vecchio): tutto come prima
        st = Stato(sessioni_viste={"Realm-Gino-1793000000": {"eventi": 2, "visto": 1}})
        (s,) = ingresso.raccogli(self.d, "201", st).sessioni
        self.assertEqual(len(s["sessione"]["events"]), 4)

    def test_fino_comprende_episodi_finiti_dopo_l_ultimo_evento(self):
        doc = doc_di(GINO)
        ultimo = max(e["t"] for e in doc["sessione"]["events"])
        doc["combattimenti"].append({"from": ultimo - 10, "to": ultimo + 30, "type": "fuga"})
        self.invia("a.json.gz", gz(doc))
        (s,) = ingresso.raccogli(self.d, "201", Stato()).sessioni
        self.assertEqual(ingresso.fino(s), ultimo + 30)
        # un episodio con orario assurdo non sposta "fino"
        s["combattimenti"].append({"from": 0, "to": 10**15, "type": "fuga"})
        self.assertEqual(ingresso.fino(s), ultimo + 30)

    def _lua_gino_grizzo(self):
        base = 1793000000
        def sess(i, char, t0, n):
            ev = " ".join('{ ["t"] = %d, ["k"] = "zone" },' % (t0 + 10 + j) for j in range(n))
            return ('{ ["id"] = "Realm-%s-%d", ["char"] = "%s", ["realm"] = "Realm", ["start"] = %d, ["stop"] = %d, '
                    '["events"] = { %s }, },' % (char, t0, char, t0, t0 + 500, ev))
        return ("DiarioGildaDB = { sessions = { %s %s } }" % (
            sess(1, "Gino", base + 50000, 1), sess(2, "Grizzo", base + 60000, 9))).encode()

    def test_principali_imparati_dal_programmino(self):
        doc = doc_di(GINO)  # Gino/Realm dal programmino 0.1.1: è un principale
        doc["app"] = "0.1.1"
        self.invia(LUA, self._lua_gino_grizzo(), autore="777")  # il .lua arriva prima del pacchetto
        self.invia("a.json.gz", gz(doc))
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertEqual(r.principali, {"gino|realm"})
        chars = sorted({s["sessione"]["char"] for s in r.sessioni})
        self.assertEqual(chars, ["Gino"])  # Grizzo (più eventi) resta fuori
        self.assertTrue(any("solo i personaggi principali" in a for a in r.avvisi), r.avvisi)

    def test_principali_dallo_stato_e_non_da_versioni_vecchie(self):
        doc = doc_di(GINO)
        doc["app"] = "0.1.0"  # la 0.1.0 inviava anche gli alt: non insegna nulla
        self.invia("a.json.gz", gz(doc))
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertEqual(r.principali, set())
        self.invia(LUA, self._lua_gino_grizzo(), autore="777")
        r = ingresso.raccogli(self.d, "201", Stato(principali=["grizzo|realm"]))
        self.assertEqual(sorted({s["sessione"]["char"] for s in r.sessioni if s.get("manuale")}), ["Grizzo"])

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

    def test_eventi_assurdi_un_avviso_per_sessione(self):
        doc = doc_di(GINO)
        doc["sessione"]["events"] += [{"t": 10**15, "k": "level", "level": 60}, {"t": 0, "k": "zone"}]
        self.invia("a.json.gz", gz(doc))
        testo = (DATI / LUA).read_text(encoding="utf-8").replace(
            '["t"] = 1793000000,', '["t"] = 0,', 1)
        self.invia(LUA, testo.encode(), autore="777")
        r = ingresso.raccogli(self.d, "201", Stato())
        self.assertEqual(len(r.sessioni), 2)
        self.assertEqual(len(r.avvisi), 2, r.avvisi)
        self.assertIn("2 eventi con orario non valido", r.avvisi[0])
        self.assertIn("a.json.gz", r.avvisi[0])
        self.assertIn("1 eventi con orario non valido", r.avvisi[1])

    def test_avvisi_con_nomi_ostili_puliti(self):
        nome = "x\n# ordini " + "a" * 300 + ".json.gz"
        self.invia(nome, gz({"formato": "\n# " + "b" * 500, "giocatore": "A", "sessione": {}}))
        r = ingresso.raccogli(self.d, "201", Stato())
        (a,) = r.avvisi
        self.assertNotIn("\n", a)
        self.assertNotIn("a" * 61, a)
        self.assertNotIn("b" * 121, a)

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
