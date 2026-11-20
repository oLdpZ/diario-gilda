import io
import json
import os
import unittest
import urllib.error
from unittest import mock

from bot.saggio import whatsapp as W

MSG = {"id": "999", "attachments": [{"url": "https://cdn.discordapp.com/x.mp4"}]}


class Risposta(io.BytesIO):
    status = 200

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False


class TestAnnuncia(unittest.TestCase):
    def setUp(self):
        self.env = mock.patch.dict(os.environ, {"WA_CRONACA_SECRET": "s3cr3t"}, clear=False)
        self.env.start()
        os.environ.pop("CANCELLIERE_URL", None)

    def tearDown(self):
        self.env.stop()

    def test_senza_secret_non_chiama(self):
        os.environ.pop("WA_CRONACA_SECRET")
        with mock.patch.object(W, "_APRI") as apri:
            self.assertFalse(W.annuncia("123", "T", MSG))
        apri.assert_not_called()

    def test_senza_allegato_non_chiama(self):
        with mock.patch.object(W, "_APRI") as apri:
            self.assertFalse(W.annuncia("123", "T", {"id": "1"}))
        apri.assert_not_called()

    def test_in_coda(self):
        with mock.patch.object(W, "_APRI", return_value=Risposta(b'{"ok":true,"queued":true}')) as apri:
            self.assertTrue(W.annuncia("123", "Cronache — Giorno 16", MSG))
        req = apri.call_args.args[0]
        self.assertEqual(req.full_url, "https://orda-bottega.oldpz1.workers.dev/wa/cronaca")
        self.assertEqual(req.get_header("Authorization"), "Bearer s3cr3t")
        self.assertEqual(json.loads(req.data), {"ref": "123", "titolo": "Cronache — Giorno 16",
                                                "video_url": "https://cdn.discordapp.com/x.mp4"})

    def test_errori_non_lanciano(self):
        err = urllib.error.HTTPError("u", 401, "no", {}, io.BytesIO(b""))
        for eccezione in (err, OSError("rete giù")):
            with mock.patch.object(W, "_APRI", side_effect=eccezione):
                self.assertFalse(W.annuncia("123", "T", MSG))


if __name__ == "__main__":
    unittest.main()
