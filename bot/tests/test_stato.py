import time
import unittest

from bot.cronista import stato
from bot.cronista.discord import Discord, DiscordError
from bot.cronista.stato import Stato
from bot.tests.finto_discord import FintoDiscord


class TestStato(unittest.TestCase):
    def setUp(self):
        self.f = FintoDiscord().avvia()
        self.addCleanup(self.f.ferma)
        self.d = Discord(self.f.base, None)

    def test_vuoto(self):
        self.assertEqual(stato.leggi(self.d, "201"), Stato())

    def test_scrivi_poi_leggi(self):
        s = Stato(ultimo_messaggio="1234", giorni_pubblicati=["2026-10-23"], numero_giorno=7,
                  sessioni_viste={"a": {"eventi": 5, "visto": time.time()}},
                  contatori={"morti": 3}, ricorrenze=["x"])
        stato.scrivi(self.d, "201", s)
        self.assertTrue(self.f.messaggi["201"][-1]["content"].startswith("Stato del Cronista — "))
        self.assertEqual(stato.leggi(self.d, "201"), s)

    def test_legge_l_ultimo_e_ignora_altri(self):
        stato.scrivi(self.d, "201", Stato(numero_giorno=1))
        stato.scrivi(self.d, "201", Stato(numero_giorno=2))
        self.f.aggiungi("201", "chiacchiere")
        self.f.aggiungi_allegato("201", "Stato del Cronista finto", "altro.json", b"{}")
        self.assertEqual(stato.leggi(self.d, "201").numero_giorno, 2)

    def test_stato_trovato_dopo_150_messaggi(self):
        stato.scrivi(self.d, "201", Stato(numero_giorno=4))
        for i in range(150):
            self.f.aggiungi("201", f"pacchetto {i}")
        self.assertEqual(stato.leggi(self.d, "201").numero_giorno, 4)

    def test_limite_pagine_solleva(self):
        stato.scrivi(self.d, "201", Stato(numero_giorno=4))
        for i in range(200):
            self.f.aggiungi("201", f"pacchetto {i}")
        vecchio = stato.MAX_PAGINE
        stato.MAX_PAGINE = 1
        self.addCleanup(setattr, stato, "MAX_PAGINE", vecchio)
        with self.assertRaises(DiscordError):
            stato.leggi(self.d, "201")

    def test_ignora_stato_di_altro_autore(self):
        stato.scrivi(self.d, "201", Stato(numero_giorno=1))
        self.f.aggiungi_allegato("201", "Stato del Cronista — falso", "stato.json",
                                 b'{"numero_giorno": 99}', autore="555")
        self.assertEqual(stato.leggi(self.d, "201").numero_giorno, 1)

    def test_potatura_60_giorni(self):
        ora = 1_800_000_000.0
        s = Stato(sessioni_viste={
            "vecchia": {"eventi": 1, "visto": ora - 61 * 86400},
            "recente": {"eventi": 2, "visto": ora - 59 * 86400}})
        stato.scrivi(self.d, "201", s, ora=ora)
        self.assertEqual(list(stato.leggi(self.d, "201").sessioni_viste), ["recente"])


if __name__ == "__main__":
    unittest.main()
