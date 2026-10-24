"""Finto server Discord (http.server in un thread) per i test."""
from __future__ import annotations

import json
import threading
from email.parser import BytesParser
from email.policy import default
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse


class _Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):  # silenzioso
        pass

    def _rispondi(self, codice: int, corpo, tipo: str = "application/json"):
        dati = corpo if isinstance(corpo, bytes) else json.dumps(corpo).encode()
        self.send_response(codice)
        self.send_header("Content-Type", tipo)
        self.send_header("Content-Length", str(len(dati)))
        self.end_headers()
        self.wfile.write(dati)

    def _registra(self, corpo: bytes = b"") -> dict:
        f = self.server.finto
        r = {"metodo": self.command, "percorso": self.path,
             "header": {k: v for k, v in self.headers.items()}, "corpo": corpo}
        f.richieste.append(r)
        return r

    def _controlli(self) -> bool:
        f = self.server.finto
        if f.errori_429 > 0:
            f.errori_429 -= 1
            self._rispondi(429, {"retry_after": 0.01, "message": "rate limited"})
            return True
        if f.forza_status:
            self._rispondi(f.forza_status, {"message": "vietato"})
            return True
        return False

    def do_GET(self):
        self._registra()
        f = self.server.finto
        if self._controlli():
            return
        u = urlparse(self.path)
        q = {k: v[0] for k, v in parse_qs(u.query).items()}
        p = u.path
        if p.startswith("/allegati/"):
            nome = p[len("/allegati/"):]
            if nome in f.allegati:
                return self._rispondi(200, f.allegati[nome], "application/octet-stream")
            return self._rispondi(404, {})
        if p == "/users/@me/guilds":
            return self._rispondi(200, f.guilds)
        parti = p.strip("/").split("/")
        if len(parti) == 3 and parti[0] == "guilds" and parti[2] == "channels":
            return self._rispondi(200, f.canali.get(parti[1], []))
        if len(parti) == 3 and parti[0] == "channels" and parti[2] == "messages":
            msgs = sorted(f.messaggi.get(parti[1], []), key=lambda m: int(m["id"]))
            lim = int(q.get("limit", 50))
            if "after" in q:
                sel = [m for m in msgs if int(m["id"]) > int(q["after"])][:lim]
            elif "before" in q:
                sel = [m for m in msgs if int(m["id"]) < int(q["before"])][-lim:]
            else:
                sel = msgs[-lim:]
            # come Discord: sempre dal più recente al più vecchio
            return self._rispondi(200, sorted(sel, key=lambda m: -int(m["id"])))
        self._rispondi(404, {})

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        corpo = self.rfile.read(n)
        r = self._registra(corpo)
        f = self.server.finto
        if self._controlli():
            return
        parti = urlparse(self.path).path.strip("/").split("/")
        if not (len(parti) == 3 and parti[0] == "channels" and parti[2] == "messages"):
            return self._rispondi(404, {})
        tipo = self.headers.get("Content-Type", "")
        allegati = []
        if tipo.startswith("multipart/form-data"):
            msg = BytesParser(policy=default).parsebytes(
                b"Content-Type: " + tipo.encode() + b"\r\n\r\n" + corpo)
            payload, parti_mp = {}, {}
            for parte in msg.iter_parts():
                nome = parte.get_param("name", header="content-disposition")
                parti_mp[nome] = parte
                if nome == "payload_json":
                    payload = json.loads(parte.get_content())
            r["multipart"] = {k: v.get_content_type() for k, v in parti_mp.items()}
            r["payload"] = payload
            fp = parti_mp.get("files[0]")
            if fp is not None:
                nomef = fp.get_filename()
                dati = fp.get_payload(decode=True)
                f.allegati[nomef] = dati
                allegati.append({"filename": nomef, "size": len(dati),
                                 "url": f"http://{self.headers['Host']}/allegati/{nomef}"})
        else:
            payload = json.loads(corpo or b"{}")
            r["payload"] = payload
        m = f.aggiungi(parti[1], payload.get("content", ""), allegati)
        m["embeds"] = payload.get("embeds", [])
        self._rispondi(200, m)


class FintoDiscord:
    """Uso: f = FintoDiscord().avvia(); ...; f.ferma()."""

    def __init__(self):
        self.guilds = [{"id": "100", "name": "Gilda"}]
        self.canali = {"100": [{"id": "200", "name": "diario-gilda", "type": 0},
                               {"id": "201", "name": "diario-servizio", "type": 0}]}
        self.messaggi: dict[str, list[dict]] = {}
        self.allegati: dict[str, bytes] = {}
        self.richieste: list[dict] = []
        self.errori_429 = 0
        self.forza_status = 0
        self._prossimo = 1000
        self._srv = None
        self._th = None

    @property
    def base(self) -> str:
        return f"http://127.0.0.1:{self._srv.server_address[1]}"

    def aggiungi(self, canale: str, contenuto: str = "", allegati: list[dict] | None = None) -> dict:
        self._prossimo += 1
        m = {"id": str(self._prossimo), "content": contenuto, "attachments": allegati or []}
        self.messaggi.setdefault(canale, []).append(m)
        return m

    def aggiungi_allegato(self, canale: str, contenuto: str, nome: str, dati: bytes) -> dict:
        self.allegati[nome] = dati
        return self.aggiungi(canale, contenuto, [{
            "filename": nome, "size": len(dati), "url": f"{self.base}/allegati/{nome}"}])

    def avvia(self) -> "FintoDiscord":
        self._srv = ThreadingHTTPServer(("127.0.0.1", 0), _Handler)
        self._srv.finto = self
        self._th = threading.Thread(target=self._srv.serve_forever, kwargs={"poll_interval": 0.01}, daemon=True)
        self._th.start()
        return self

    def ferma(self) -> None:
        self._srv.shutdown()
        self._srv.server_close()
        self._th.join()
