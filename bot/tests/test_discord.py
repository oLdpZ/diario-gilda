import os
import unittest

import bot.cronista.discord as mod
from bot.cronista.discord import Discord, DiscordError
from bot.tests.finto_discord import FintoDiscord


class TestDiscord(unittest.TestCase):
    def setUp(self):
        self.f = FintoDiscord().avvia()
        self.addCleanup(self.f.ferma)

    def cl(self, token=None):
        return Discord(self.f.base, token)

    def test_server_unico_e_per_nome(self):
        self.assertEqual(self.cl().server(None)["id"], "100")
        self.assertEqual(self.cl().server("Gilda")["id"], "100")
        self.assertEqual(self.cl().server("100")["name"], "Gilda")

    def test_server_ambiguo(self):
        self.f.guilds.append({"id": "101", "name": "Altro"})
        with self.assertRaises(DiscordError):
            self.cl().server(None)

    def test_canale_per_nome(self):
        self.assertEqual(self.cl().canale("100", "diario-servizio"), "201")
        with self.assertRaises(DiscordError):
            self.cl().canale("100", "nope")

    def test_messaggi_dopo_paginati_dal_piu_vecchio(self):
        for i in range(250):
            self.f.aggiungi("201", f"m{i}")
        tutti = self.cl().messaggi_dopo("201", None)
        self.assertEqual([m["content"] for m in tutti], [f"m{i}" for i in range(250)])
        dopo = self.f.messaggi["201"][49]["id"]
        resto = self.cl().messaggi_dopo("201", dopo)
        self.assertEqual(len(resto), 200)
        self.assertEqual(resto[0]["content"], "m50")
        self.assertEqual(resto[-1]["content"], "m249")

    def test_ultimi(self):
        for i in range(150):
            self.f.aggiungi("201", f"m{i}")
        u = self.cl().ultimi("201", 120)
        self.assertEqual(len(u), 120)
        self.assertEqual(u[-1]["content"], "m149")
        self.assertEqual(u[0]["content"], "m30")

    def test_429_poi_ok(self):
        self.f.errori_429 = 1
        self.assertEqual(self.cl().server(None)["id"], "100")
        self.assertEqual(len(self.f.richieste), 2)

    def test_429_oltre_tre_tentativi(self):
        self.f.errori_429 = 5
        with self.assertRaises(DiscordError) as c:
            self.cl().server(None)
        self.assertEqual(c.exception.status, 429)
        self.assertEqual(len(self.f.richieste), 3)

    def test_token_nell_header_e_mai_nell_errore(self):
        self.cl("segreto").server(None)
        self.assertEqual(self.f.richieste[-1]["header"]["Authorization"], "Bot segreto")
        self.f.forza_status = 403
        with self.assertRaises(DiscordError) as c:
            self.cl("segreto").server(None)
        self.assertEqual(c.exception.status, 403)
        self.assertNotIn("segreto", str(c.exception))
        self.assertNotIn("segreto", repr(c.exception))

    def test_senza_token_nessun_header(self):
        vecchio = os.environ.pop("DISCORD_BOT_TOKEN", None)
        if vecchio is not None:
            self.addCleanup(os.environ.__setitem__, "DISCORD_BOT_TOKEN", vecchio)
        self.cl().server(None)
        self.assertNotIn("Authorization", self.f.richieste[-1]["header"])
        self.assertIn("DiarioGilda-Cronista", self.f.richieste[-1]["header"]["User-Agent"])

    def test_invia_json_con_embed(self):
        r = self.cl().invia("200", "ciao", embed=[{"title": "t"}])
        rich = self.f.richieste[-1]
        self.assertEqual(rich["payload"]["allowed_mentions"], {"parse": []})
        self.assertEqual(rich["payload"]["embeds"], [{"title": "t"}])
        self.assertEqual(r["content"], "ciao")

    def test_invia_con_file_multipart(self):
        self.cl().invia("201", "x", file=("stato.json", b'{"a": 1}'))
        rich = self.f.richieste[-1]
        self.assertEqual(rich["multipart"]["files[0]"], "application/octet-stream")
        self.assertEqual(rich["multipart"]["payload_json"], "application/json")
        self.assertEqual(rich["payload"]["allowed_mentions"], {"parse": []})
        self.assertEqual(self.f.allegati["stato.json"], b'{"a": 1}')

    def test_scarica(self):
        m = self.f.aggiungi_allegato("201", "c", "a.bin", b"\x00\x01dati")
        self.assertEqual(self.cl("tok").scarica(m["attachments"][0]["url"]), b"\x00\x01dati")

    def test_scarica_troppo_grande(self):
        vecchio = mod.CAP_ALLEGATO
        mod.CAP_ALLEGATO = 10
        self.addCleanup(setattr, mod, "CAP_ALLEGATO", vecchio)
        m = self.f.aggiungi_allegato("201", "c", "g.bin", b"x" * 50)
        with self.assertRaises(DiscordError):
            self.cl().scarica(m["attachments"][0]["url"])


if __name__ == "__main__":
    unittest.main()
