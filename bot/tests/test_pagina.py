import unittest

from bot.cronista import pagina


def _totale(msg: list[dict]) -> int:
    return sum(len(e.get("title", "")) + len(e.get("description", ""))
               + len(e.get("footer", {}).get("text", "")) for e in msg)


class TestPagina(unittest.TestCase):
    def test_breve(self):
        msgs = pagina.embed("# Cronache della Gilda — Giorno 1, Era della Rinascita\n\nNarrano le genti.\n")
        self.assertEqual(len(msgs), 1)
        e = msgs[0][0]
        self.assertEqual(e["title"], "Cronache della Gilda — Giorno 1, Era della Rinascita")
        self.assertEqual(e["description"], "Narrano le genti.")
        self.assertEqual(e["color"], 0xC8A15A)
        self.assertEqual(e["footer"], {"text": "Il Cronista della Gilda"})

    def test_lunga_divisa_senza_spezzare(self):
        parole = [f"parola{i}" for i in range(1200)]
        paragrafi = [" ".join(parole[i:i + 100]) for i in range(0, 1200, 100)]
        testo = "# Titolo lungo\n\n" + "\n\n".join(paragrafi)
        corpo = "\n\n".join(paragrafi)
        self.assertGreater(len(corpo), 9000)
        msgs = pagina.embed(testo)
        embeds = [e for m in msgs for e in m]
        self.assertGreater(len(embeds), 2)
        self.assertEqual(embeds[0]["title"], "Titolo lungo")
        self.assertIn("footer", embeds[0])
        for m in msgs:
            self.assertLessEqual(len(m), 10)
            self.assertLessEqual(_totale(m), 6000)
        for e in embeds:
            self.assertLessEqual(len(e["description"]), 4096)
            self.assertEqual(e["color"], 0xC8A15A)
        # nessuna parola spezzata e nulla perso
        ricomposto = " ".join(e["description"] for e in embeds).split()
        self.assertEqual(ricomposto, corpo.split())

    def test_paragrafo_enorme_diviso_a_parole(self):
        corpo = " ".join(["drago"] * 2000)  # un solo paragrafo di 11999 caratteri
        msgs = pagina.embed("# T\n\n" + corpo)
        embeds = [e for m in msgs for e in m]
        for e in embeds:
            self.assertLessEqual(len(e["description"]), 4096)
            self.assertTrue(set(e["description"].split()) == {"drago"})
        self.assertEqual(sum(len(e["description"].split()) for e in embeds), 2000)

    def test_titolo_troppo_lungo_e_senza_titolo(self):
        e = pagina.embed("# " + "a " * 200 + "\n\ntesto")[0][0]
        self.assertLessEqual(len(e["title"]), 256)
        e = pagina.embed("solo testo")[0][0]
        self.assertNotIn("title", e)
        self.assertEqual(e["description"], "solo testo")

    def test_niente_menzioni(self):
        e = pagina.embed("# T\n\nSaluti a @everyone e @Gino")[0][0]
        self.assertNotIn("@everyone", e["description"])
        self.assertNotIn("@Gino", e["description"])

    def test_vuota(self):
        with self.assertRaises(ValueError):
            pagina.embed("  \n\n ")


if __name__ == "__main__":
    unittest.main()
