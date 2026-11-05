import unittest
from datetime import datetime, timedelta, timezone

from bot.saggio import testo as T

IO = "999"
ORA = datetime(2026, 11, 6, 7, 40, tzinfo=timezone.utc)


def msg(i, ore_fa=1.0, autore=IO, embeds=None, file=None):
    m = {"id": str(i), "author": {"id": autore},
         "timestamp": (ORA - timedelta(hours=ore_fa)).isoformat().replace("+00:00", "Z"),
         "embeds": embeds or [], "attachments": []}
    if file:
        m["attachments"] = [{"filename": file}]
    return m


def emb(desc, titolo=None):
    e = {"description": desc}
    if titolo:
        e["title"] = titolo
        e["footer"] = {"text": T.PIEDE}
    return e


class TestPagine(unittest.TestCase):
    def test_pagina_su_piu_messaggi(self):
        ms = [msg(1, embeds=[emb("Primo.", "Cronache della Gilda — Giorno 2, Era della Rinascita")]),
              msg(2, embeds=[emb("Secondo."), emb("**Bilancio della giornata**\nnumeri")])]
        (p,) = T.pagine(ms, IO)
        self.assertEqual(p["id"], "1")
        self.assertIn("Secondo.", p["testo"])

    def test_altri_messaggi_non_sono_pagine(self):
        ms = [msg(1, embeds=[emb("Guida", "Come funziona il diario")]),       # niente «Cronache» né piede
              msg(2, autore="123", embeds=[emb("x", "Cronache finte")]),     # non del bot
              msg(3, embeds=[emb("Avviso senza titolo")])]                   # seguito senza pagina
        self.assertEqual(T.pagine(ms, IO), [])

    def test_messaggio_di_altri_chiude_la_pagina(self):
        ms = [msg(1, embeds=[emb("A.", "Cronache della Gilda — Giorno 2")]),
              msg(2, autore="123", embeds=[emb("intruso")]),
              msg(3, embeds=[emb("non è seguito")])]
        (p,) = T.pagine(ms, IO)
        self.assertNotIn("non è seguito", p["testo"])

    def test_da_narrare_salta_fatte_e_vecchie(self):
        ms = [msg(1, ore_fa=30, embeds=[emb("vecchia", "Cronache della Gilda — Giorno 1")]),
              msg(2, ore_fa=2, embeds=[emb("nuova", "Cronache della Gilda — Giorno 2")]),
              msg(3, ore_fa=1, embeds=[emb("altra", "Le Cronache dei primi giorni")]),
              msg(4, ore_fa=0.5, file="cronaca-3.mp4")]
        self.assertEqual([p["id"] for p in T.da_narrare(ms, IO, ORA)], ["2"])

    def test_video_di_altri_non_conta(self):
        ms = [msg(2, ore_fa=2, embeds=[emb("nuova", "Cronache della Gilda — Giorno 2")]),
              msg(4, ore_fa=1, autore="123", file="cronaca-2.mp4")]
        self.assertEqual([p["id"] for p in T.da_narrare(ms, IO, ORA)], ["2"])


class TestTesto(unittest.TestCase):
    def test_pulisci_toglie_bilancio_e_markdown(self):
        t = "Fu **grande** e *strano*.\n\n> citazione\n\n**Bilancio della giornata**\n**Sciatalgia** — livello 1 → 6"
        p = T.pulisci(t)
        self.assertEqual(p, "Fu grande e strano.\n\ncitazione")

    def test_frasi_con_virgolette(self):
        self.assertEqual(T.frasi("Fu là che portò a compimento «Marla's Last Wish». Cadde. Tornò!"),
                         ["Fu là che portò a compimento «Marla's Last Wish».", "Cadde.", "Tornò!"])

    def test_blocchi_uniscono_frasi_corte(self):
        bl = T.blocchi("Cronache della Gilda — Giorno 2, Era della Rinascita",
                       "Nessun corno, nessuno stendardo. Soltanto la nebbia della tomba e una lama.\n\nCadde.")
        self.assertEqual(bl[0], ["Cronache della Gilda. Giorno 2, Era della Rinascita."])
        self.assertEqual(bl[1], ["Nessun corno, nessuno stendardo. Soltanto la nebbia della tomba e una lama."])
        self.assertEqual(bl[2], ["Cadde."])  # da sola nel paragrafo: resta da sola

    def test_frase_corta_finale_va_con_la_precedente(self):
        bl = T.blocchi("Cronache", "Una frase abbastanza lunga da stare da sola, davvero. Cadde.")
        self.assertEqual(bl[1], ["Una frase abbastanza lunga da stare da sola, davvero. Cadde."])

    def test_pezzi_sottotitoli(self):
        t = "parola " * 40
        for p in T.pezzi(t):
            self.assertLessEqual(len(p), T.MAX_SOTTOTITOLO)
        self.assertEqual(" ".join(T.pezzi(t)), t.strip())

    def test_srt(self):
        tempi = [{"testo": "Prima frase.", "da": 0.0, "a": 2.0}, {"testo": "Seconda.", "da": 2.5, "a": 4.0}]
        s = T.srt(tempi, 3.5)
        self.assertIn("00:00:03,500 -->", s)
        self.assertIn("Seconda.", s)


class TestBocca(unittest.TestCase):
    RATE = 1000

    def suono(self, *pezzi):
        """pezzi: (secondi, forte?) -> campioni (onda quadra o silenzio)."""
        out = []
        for sec, forte in pezzi:
            out += [(3000 if i % 2 else -3000) if forte else 0 for i in range(int(sec * self.RATE))]
        return out

    def test_virgola_chiude_la_bocca_respiro_corto_no(self):
        c = self.suono((0.2, False), (0.5, True), (0.3, False), (0.4, True), (0.06, False), (0.4, True), (0.2, False))
        iv = T.parlato(c, self.RATE)
        self.assertEqual(len(iv), 2)                       # la pausa di 0,3 s separa, quella di 0,06 no
        self.assertAlmostEqual(iv[0][0], 0.2, delta=0.03)
        self.assertAlmostEqual(iv[0][1], 0.7, delta=0.03)
        self.assertAlmostEqual(iv[1][0], 1.0, delta=0.03)
        self.assertAlmostEqual(iv[1][1], 1.86, delta=0.03)

    def test_silenzio_e_schiocchi(self):
        self.assertEqual(T.parlato([0] * 500, self.RATE), [])
        c = self.suono((0.5, True), (0.3, False), (0.03, True), (0.3, False))
        self.assertEqual(len(T.parlato(c, self.RATE)), 1)  # lo schiocco di 30 ms non apre la bocca

    def test_maschera_segue_la_voce(self):
        tempi = [{"testo": "x", "da": 0.0, "a": 2.0, "voce": [(0.0, 0.8), (1.2, 2.0)]}]
        m = T.maschera(tempi, 1.0, 4.0, fps=10, rampa=2)
        self.assertEqual(len(m), 40)
        self.assertEqual(m[0], 0)                  # sigla: bocca chiusa
        self.assertEqual(m[14], 255)               # 1,45 s: parla
        self.assertEqual(m[21], 0)                 # 2,15 s: la virgola, bocca chiusa
        self.assertEqual(m[27], 255)               # 2,75 s: parla di nuovo
        self.assertEqual(m[-1], 0)                 # coda
        self.assertTrue(all(abs(a - b) <= 128 for a, b in zip(m, m[1:])))  # niente scatti

    def test_maschera_senza_intervalli_usa_il_blocco(self):
        m = T.maschera([{"testo": "x", "da": 0.0, "a": 1.0}], 0.0, 2.0, fps=10, rampa=1)
        self.assertEqual(m[5], 255)
        self.assertEqual(m[15], 0)


if __name__ == "__main__":
    unittest.main()
