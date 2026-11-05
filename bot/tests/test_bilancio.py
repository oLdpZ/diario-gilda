import unittest

from bot.cronista import bilancio


def sessione(giocatore, eventi, episodi=(), char=None, level=None, start=1793000000):
    se = {"id": f"R-{giocatore}-{start}", "char": char or giocatore, "start": start, "events": eventi}
    if level is not None:
        se["level"] = level
    return {"giocatore": giocatore, "interrotta": False, "troncata": 0, "combattimenti": list(episodi),
            "sessione": se}


class TestBilancio(unittest.TestCase):
    def test_serata_di_sciatalgia(self):
        ev = [{"t": 1, "k": "zone", "z": "Tirisfal Glades", "s": "Deathknell"},
              {"t": 2, "k": "quest", "action": "done", "title": "Rude Awakening", "s": "Deathknell"},
              {"t": 3, "k": "level", "level": 2}, {"t": 4, "k": "level", "level": 6},
              {"t": 5, "k": "quest", "action": "abandon", "title": "Nope"},
              {"t": 6, "k": "death"}, {"t": 7, "k": "death"},
              {"t": 8, "k": "zone", "z": "Tirisfal Glades", "s": "Solliden Farmstead"}]
        eps = [{"from": 1, "to": 2, "kind": "combat", "kills": [{"name": "Rat", "n": 3}]},
               {"from": 3, "to": 4, "kind": "combat", "kills": [{"name": "Duskbat", "n": 2}]}]
        (v,) = bilancio.righe([sessione("Sciatalgia Fulminante", ev, eps, char="Sciatalgia", level=1)])
        self.assertEqual((v["da"], v["a"], v["missioni"], v["nemici"], v["morti"]),
                         (1, 6, ["Rude Awakening"], 5, 2))
        self.assertEqual(v["luoghi"], ["Deathknell", "Solliden Farmstead"])
        self.assertIn("**Sciatalgia** — livello 1 → 6 · 1 missione (Rude Awakening) · 5 nemici · 2 morti"
                      " · Deathknell, Solliden Farmstead", bilancio.md([v]))
        self.assertTrue(bilancio.md([v]).startswith("**Bilancio della giornata**"))

    def test_senza_episodi_niente_nemici(self):
        (v,) = bilancio.righe([sessione("Mara", [{"t": 1, "k": "level", "level": 3}], level=2)])
        self.assertIsNone(v["nemici"])
        self.assertNotIn("nemici", bilancio.md([v]))

    def test_nemici_solo_dagli_episodi_di_combattimento(self):  # l'episodio di morte ripete le uccisioni
        eps = [{"from": 1, "to": 2, "kind": "combat", "kills": [{"name": "Wolf", "n": 1}]},
               {"from": 1, "to": 3, "kind": "death", "kills": [{"name": "Wolf", "n": 1}]}]
        (v,) = bilancio.righe([sessione("Mara", [{"t": 3, "k": "death"}], eps)])
        self.assertEqual(v["nemici"], 1)
        self.assertIn("1 nemico", bilancio.md([v]))
        solo_morte = [{"from": 1, "to": 3, "kind": "death", "kills": [{"name": "Wolf", "n": 1}]}]
        (v,) = bilancio.righe([sessione("Mara", [{"t": 3, "k": "death"}], solo_morte)])
        self.assertIsNone(v["nemici"])

    def test_missioni_contate_tutte_titoli_senza_doppioni(self):
        ev = [{"t": 1, "k": "quest", "action": "done", "title": "A"},
              {"t": 2, "k": "quest", "action": "done", "title": "A"},
              {"t": 3, "k": "quest", "action": "done"},
              {"t": 4, "k": "quest", "action": "done", "title": "B"}]
        (v,) = bilancio.righe([sessione("Mara", ev)])
        self.assertEqual(v["missioni"], ["A", "B"])
        self.assertIn("4 missioni (A, B)", bilancio.md([v]))
        ev = [{"t": 1, "k": "quest", "action": "done", "title": "A"},
              {"t": 2, "k": "quest", "action": "done", "title": "A"},
              {"t": 3, "k": "quest", "action": "done"}]
        (v,) = bilancio.righe([sessione("Mara", ev)])
        self.assertIn("3 missioni (A)", bilancio.md([v]))
        (v,) = bilancio.righe([sessione("Mara", [{"t": 3, "k": "quest", "action": "done"}])])
        self.assertIn("**Mara** — 1 missione\n", bilancio.md([v]))

    def test_ritardo_riga_a_parte(self):
        a, b = sessione("Gino", [], start=1793834441), sessione("Gino", [], start=1793834441 - 86400)
        voci = bilancio.righe([a, b], tardi={id(b)})
        self.assertEqual([v["ritardo"] for v in voci], [None, "2026-11-03"])
        self.assertIn("notizie dal 3 novembre", bilancio.md(voci))

    def test_due_personaggi_due_righe_e_missioni_oltre_8(self):
        ev = [{"t": i, "k": "quest", "action": "done", "title": f"Q{i}"} for i in range(1, 11)]
        voci = bilancio.righe([sessione("Zeta", [], char="Zed-Reame"), sessione("Zeta", ev, char="Abe")])
        self.assertEqual([v["char"] for v in voci], ["Abe", "Zed"])
        testo = bilancio.md(voci)
        self.assertIn("10 missioni (Q1, Q2, Q3, Q4, Q5, Q6, Q7, Q8, e altre 2)", testo)
        self.assertIn("**Zed**\n", testo)

    def test_testi_strani(self):
        ev = [{"t": 1, "k": "quest", "action": "done", "title": "# @everyone\nciao " + "x" * 100},
              {"t": 2, "k": "quest", "action": "done", "title": 5}, "rotto", None]
        voci = bilancio.righe([sessione("Mara", ev), "no", {"giocatore": 3}])
        righe_md = bilancio.md(voci).splitlines()
        self.assertEqual(len(righe_md), 3)
        self.assertTrue(righe_md[2].startswith("**Mara**"))
        self.assertNotIn("#", righe_md[2])
        self.assertEqual(len(voci[0]["missioni"][0]), 60)


if __name__ == "__main__":
    unittest.main()
