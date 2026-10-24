"""Lettore del sottoinsieme Lua delle SavedVariables (porta del parser Go svparse).

Tabelle -> list se le chiavi sono interi consecutivi da 1 (`{}` -> []), altrimenti
dict con chiavi stringa (anche quelle intere, in decimale). Interi in base 10 -> int,
il resto -> float. Tollera BOM e CRLF; profondita' massima 200.
"""
from __future__ import annotations

MAX_PROFONDITA = 200
_ESC = {"n": 10, "t": 9, "r": 13, "a": 7, "b": 8, "f": 12, "v": 11}


def _inizio_ident(c: int) -> bool:
    return c == 95 or 65 <= c <= 90 or 97 <= c <= 122


def _cifra(c: int) -> bool:
    return 48 <= c <= 57


class _Parser:
    def __init__(self, src: bytes):
        self.s = src
        self.p = 0
        self.riga = 1
        self.prof = 0

    def err(self, msg: str) -> ValueError:
        return ValueError(f"riga {self.riga}: {msg}")

    def eof(self) -> bool:
        return self.p >= len(self.s)

    def peek(self) -> int:
        return self.s[self.p]

    def salta(self) -> None:
        s = self.s
        while self.p < len(s):
            c = s[self.p]
            if c == 10:
                self.riga += 1
                self.p += 1
            elif c in (32, 9, 13):
                self.p += 1
            elif c == 45 and s[self.p + 1:self.p + 2] == b"-":
                self.p += 2
                liv = self.apertura_lunga()
                if liv is not None:
                    self.corpo_lungo(liv)
                    continue
                while self.p < len(s) and s[self.p] != 10:
                    self.p += 1
            else:
                return

    def apertura_lunga(self) -> int | None:
        i = self.p
        if self.s[i:i + 1] != b"[":
            return None
        i += 1
        liv = 0
        while self.s[i:i + 1] == b"=":
            liv += 1
            i += 1
        if self.s[i:i + 1] == b"[":
            self.p = i + 1
            return liv
        return None

    def corpo_lungo(self, liv: int) -> str:
        chiusura = b"]" + b"=" * liv + b"]"
        i = self.s.find(chiusura, self.p)
        if i < 0:
            raise self.err("stringa lunga non chiusa")
        corpo = self.s[self.p:i]
        self.riga += corpo.count(b"\n")
        self.p = i + len(chiusura)
        if corpo.startswith(b"\r"):
            corpo = corpo[1:]
        if corpo.startswith(b"\n"):
            corpo = corpo[1:]
        return corpo.decode("utf-8", "replace")

    def ident(self) -> str:
        if self.eof() or not _inizio_ident(self.peek()):
            raise self.err("carattere inatteso")
        a = self.p
        while not self.eof() and (_inizio_ident(self.peek()) or _cifra(self.peek())):
            self.p += 1
        return self.s[a:self.p].decode("ascii")

    def valore(self):
        self.salta()
        if self.eof():
            raise self.err("valore mancante, file troncato")
        c = self.peek()
        if c == 123:  # {
            self.p += 1
            return self.tabella()
        if c in (34, 39):
            return self.quotata()
        if c == 91:  # [
            liv = self.apertura_lunga()
            if liv is None:
                raise self.err("carattere inatteso '['")
            return self.corpo_lungo(liv)
        if c in (45, 43, 46) or _cifra(c):
            return self.numero()
        if _inizio_ident(c):
            i = self.ident()
            if i == "true":
                return True
            if i == "false":
                return False
            if i == "nil":
                return None
            raise self.err(f"identificatore inatteso {i!r}")
        raise self.err("carattere inatteso")

    def numero(self):
        a = self.p
        if self.peek() in (45, 43):
            self.p += 1
        esa = False
        flt = False
        if self.s[self.p:self.p + 2].lower() == b"0x":
            esa = True
            self.p += 2
        while not self.eof():
            c = self.peek()
            if _cifra(c) or (esa and (65 <= c <= 70 or 97 <= c <= 102)):
                pass
            elif c == 46:
                flt = True
            elif not esa and c in (69, 101):
                flt = True
                if self.s[self.p + 1:self.p + 2] in (b"-", b"+"):
                    self.p += 1
            else:
                break
            self.p += 1
        txt = self.s[a:self.p].decode("ascii")
        if not flt:
            try:
                n = int(txt, 16) if esa else int(txt, 10)
                if -(2 ** 63) <= n < 2 ** 63:
                    return n
            except ValueError:
                pass
        try:
            return float(txt)
        except ValueError:
            raise self.err(f"numero non valido {txt!r}") from None

    def quotata(self) -> str:
        q = self.peek()
        self.p += 1
        out = bytearray()
        while True:
            if self.eof():
                raise self.err("stringa non chiusa")
            c = self.peek()
            self.p += 1
            if c == q:
                return out.decode("utf-8", "replace")
            if c == 10:
                self.riga += 1
                raise self.err("stringa non chiusa")
            if c != 92:
                out.append(c)
                continue
            if self.eof():
                raise self.err("stringa non chiusa")
            e = self.peek()
            self.p += 1
            ch = chr(e)
            if ch in _ESC:
                out.append(_ESC[ch])
            elif e == 10:
                self.riga += 1
                out.append(10)
            elif ch == "x":
                esa = self.s[self.p:self.p + 2]
                try:
                    if len(esa) < 2:
                        raise ValueError
                    out.append(int(esa.decode("ascii"), 16))
                except (ValueError, UnicodeDecodeError):
                    raise self.err("escape \\x non valido") from None
                self.p += 2
            elif _cifra(e):
                n = e - 48
                for _ in range(2):
                    if not self.eof() and _cifra(self.peek()):
                        n = n * 10 + self.peek() - 48
                        self.p += 1
                if n > 255:
                    raise self.err("escape decimale fuori intervallo")
                out.append(n)
            else:
                out.append(e)

    def lunga_avanti(self) -> bool:
        salva = self.p
        ok = self.apertura_lunga() is not None
        self.p = salva
        return ok

    def campo_nominato(self) -> bool:
        i = self.p
        s = self.s
        while i < len(s) and (_inizio_ident(s[i]) or _cifra(s[i])):
            i += 1
        while i < len(s) and s[i] in (32, 9, 13, 10):
            i += 1
        return i < len(s) and s[i] == 61 and s[i + 1:i + 2] != b"="

    def tabella(self):
        self.prof += 1
        try:
            if self.prof > MAX_PROFONDITA:
                raise self.err("annidamento troppo profondo")
            return self._tabella()
        finally:
            self.prof -= 1

    def _tabella(self):
        m: dict[str, object] = {}
        n = 0
        simile_lista = True
        interi: set[int] = set()
        massimo = 0
        while True:
            self.salta()
            if self.eof():
                raise self.err("tabella non chiusa, file troncato")
            c = self.peek()
            if c == 125:  # }
                self.p += 1
                break
            if c in (44, 59):
                self.p += 1
                continue
            chiave = None
            if c == 91 and not self.lunga_avanti():
                self.p += 1
                chiave = self.valore()
                self.salta()
                if self.eof() or self.peek() != 93:
                    raise self.err("atteso ']'")
                self.p += 1
            elif _inizio_ident(c) and self.campo_nominato():
                chiave = self.ident()
            if chiave is not None:
                self.salta()
                if self.eof() or self.peek() != 61:
                    raise self.err("atteso '='")
                self.p += 1
            v = self.valore()
            if chiave is None:
                n += 1
                chiave = n
            if v is None:
                continue
            if isinstance(chiave, bool) or chiave is None:
                raise self.err("chiave di tabella non supportata")
            if isinstance(chiave, float) and chiave.is_integer():
                chiave = int(chiave)
            if isinstance(chiave, int):
                m[str(chiave)] = v
                if chiave >= 1:
                    interi.add(chiave)
                    massimo = max(massimo, chiave)
                else:
                    simile_lista = False
            elif isinstance(chiave, (str, float)):
                m[str(chiave)] = v
                simile_lista = False
            else:
                raise self.err("chiave di tabella non supportata")
        if simile_lista and len(interi) == massimo and len(m) == len(interi):
            return [m[str(i)] for i in range(1, massimo + 1)]
        return m


def carica(testo: str) -> dict:
    """Legge le assegnazioni globali `Nome = valore` e le restituisce in un dict."""
    src = testo.encode("utf-8")
    if src.startswith(b"\xef\xbb\xbf"):
        src = src[3:]
    p = _Parser(src)
    out: dict = {}
    while True:
        p.salta()
        if p.eof():
            return out
        if p.peek() == 59:
            p.p += 1
            continue
        nome = p.ident()
        p.salta()
        if p.eof() or p.peek() != 61:
            raise p.err(f"atteso '=' dopo {nome!r}")
        p.p += 1
        out[nome] = p.valore()
