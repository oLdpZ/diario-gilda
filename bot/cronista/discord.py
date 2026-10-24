"""Client REST minimale per Discord v10 (solo libreria standard)."""
from __future__ import annotations

import json
import os
import time
import urllib.error
import urllib.request
import uuid

BASE_PREDEFINITA = "https://discord.com/api/v10"
USER_AGENT = "DiarioGilda-Cronista (https://github.com/oLdpZ/diario-gilda, 0.1)"
MAX_TENTATIVI = 3
ATTESA_MAX = 30.0
CAP_ALLEGATO = 16 * 1024 * 1024


class DiscordError(Exception):
    """Errore HTTP/di protocollo; il messaggio non contiene mai il token."""

    def __init__(self, status: int, testo: str):
        self.status = status
        self.testo = testo
        super().__init__(f"Discord HTTP {status}: {testo}")


def _snowflake(m: dict) -> int:
    return int(m["id"])


class Discord:
    def __init__(self, base: str | None = None, token: str | None = None):
        self.base = (base or os.environ.get("CRONISTA_API") or BASE_PREDEFINITA).rstrip("/")
        # Nel cloud il token manca: l'header lo aggiunge il proxy.
        self._token = token if token is not None else os.environ.get("DISCORD_BOT_TOKEN")
        self._attesa = time.sleep  # sostituibile nei test

    # --- basso livello -------------------------------------------------
    def _pulisci(self, testo: str) -> str:
        if self._token:
            testo = testo.replace(self._token, "***")
        return testo[:500]

    def _richiesta(self, metodo: str, url: str, corpo: bytes | None = None,
                   tipo: str | None = None, auth: bool = True, cap: int | None = None) -> bytes:
        for tentativo in range(1, MAX_TENTATIVI + 1):
            req = urllib.request.Request(url, data=corpo, method=metodo)
            req.add_header("User-Agent", USER_AGENT)
            if tipo:
                req.add_header("Content-Type", tipo)
            if auth and self._token:
                req.add_header("Authorization", f"Bot {self._token}")
            try:
                with urllib.request.urlopen(req, timeout=30) as r:
                    if cap is None:
                        return r.read()
                    dati = r.read(cap + 1)
                    if len(dati) > cap:
                        raise DiscordError(413, "allegato troppo grande")
                    return dati
            except urllib.error.HTTPError as e:
                testo = e.read().decode("utf-8", "replace")
                e.close()
                if e.code == 429 and tentativo < MAX_TENTATIVI:
                    try:
                        attesa = float(json.loads(testo).get("retry_after", 1))
                    except (ValueError, AttributeError):
                        attesa = 1.0
                    self._attesa(min(max(attesa, 0.0), ATTESA_MAX))
                    continue
                raise DiscordError(e.code, self._pulisci(testo)) from None
            except urllib.error.URLError as e:
                raise DiscordError(0, self._pulisci(str(e.reason))) from None
        raise DiscordError(429, "troppi tentativi")  # irraggiungibile

    def _json(self, percorso: str):
        return json.loads(self._richiesta("GET", self.base + percorso))

    # --- API -----------------------------------------------------------
    def server(self, id_o_nome: str | None) -> dict:
        guilds = self._json("/users/@me/guilds")
        if id_o_nome is None:
            if len(guilds) != 1:
                raise DiscordError(0, f"il bot è in {len(guilds)} server: indicare 'server' in config")
            return guilds[0]
        for g in guilds:
            if g["id"] == str(id_o_nome) or g.get("name") == id_o_nome:
                return g
        raise DiscordError(404, f"server non trovato: {id_o_nome}")

    def canale(self, server_id: str, nome: str) -> str:
        for c in self._json(f"/guilds/{server_id}/channels"):
            if c.get("name") == nome and c.get("type", 0) == 0:
                return c["id"]
        raise DiscordError(404, f"canale non trovato: {nome}")

    def messaggi_dopo(self, canale_id: str, dopo: str | None) -> list[dict]:
        """Tutti i messaggi più recenti di `dopo` (o tutti se None), dal più vecchio."""
        out: dict[str, dict] = {}
        if dopo is None:
            prima = None
            while True:
                q = "limit=100" + (f"&before={prima}" if prima else "")
                pag = self._json(f"/channels/{canale_id}/messages?{q}")
                if not pag:
                    break
                for m in pag:
                    out[m["id"]] = m
                prima = str(min(_snowflake(m) for m in pag))
                if len(pag) < 100:
                    break
        else:
            cursore = str(dopo)
            while True:
                pag = self._json(f"/channels/{canale_id}/messages?limit=100&after={cursore}")
                if not pag:
                    break
                for m in pag:
                    out[m["id"]] = m
                cursore = str(max(_snowflake(m) for m in pag))
                if len(pag) < 100:
                    break
        return sorted(out.values(), key=_snowflake)

    def ultimi(self, canale_id: str, n: int = 100) -> list[dict]:
        """Gli ultimi `n` messaggi, dal più vecchio al più recente."""
        out: dict[str, dict] = {}
        prima = None
        while len(out) < n:
            q = f"limit={min(100, n - len(out))}" + (f"&before={prima}" if prima else "")
            pag = self._json(f"/channels/{canale_id}/messages?{q}")
            if not pag:
                break
            for m in pag:
                out[m["id"]] = m
            prima = str(min(_snowflake(m) for m in pag))
        return sorted(out.values(), key=_snowflake)

    def scarica(self, url: str) -> bytes:
        """Scarica un allegato (max 16 MB); niente Authorization verso host esterni."""
        return self._richiesta("GET", url, auth=url.startswith(self.base), cap=CAP_ALLEGATO)

    def invia(self, canale_id: str, contenuto: str = "", embed: list[dict] | None = None,
              file: tuple[str, bytes] | None = None) -> dict:
        payload: dict = {"content": contenuto, "allowed_mentions": {"parse": []}}
        if embed:
            payload["embeds"] = embed
        url = f"{self.base}/channels/{canale_id}/messages"
        pj = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        if file is None:
            risp = self._richiesta("POST", url, pj, "application/json")
        else:
            nome, dati = file
            conf = uuid.uuid4().hex
            corpo = (
                f'--{conf}\r\nContent-Disposition: form-data; name="payload_json"\r\n'
                f"Content-Type: application/json\r\n\r\n"
            ).encode() + pj + (
                f'\r\n--{conf}\r\nContent-Disposition: form-data; name="files[0]"; '
                f'filename="{nome}"\r\nContent-Type: application/octet-stream\r\n\r\n'
            ).encode() + dati + f"\r\n--{conf}--\r\n".encode()
            risp = self._richiesta("POST", url, corpo, f"multipart/form-data; boundary={conf}")
        return json.loads(risp)
