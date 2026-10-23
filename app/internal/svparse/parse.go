// Package svparse legge i file SavedVariables di WoW (sottoinsieme di Lua:
// assegnazioni globali con tabelle, stringhe, numeri, booleani, nil).
package svparse

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Parse legge le assegnazioni globali `Nome = valore` e le restituisce in una mappa.
//
// Mapping dei valori: tabelle -> []any se le chiavi sono interi consecutivi da 1
// (posizionali inclusi), altrimenti map[string]any (chiavi intere come stringhe
// decimali). Una tabella vuota `{}` diventa un []any vuoto (non nil): un gruppo
// senza membri e' `members = {}` e deve risultare una lista vuota.
// Numeri: senza '.' e senza esponente -> int64, altrimenti float64.
// Tollera BOM UTF-8 e CRLF.
func Parse(src []byte) (map[string]any, error) {
	src = bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	p := &parser{src: src, line: 1}
	out := map[string]any{}
	for {
		if err := p.skip(); err != nil {
			return nil, err
		}
		if p.eof() {
			return out, nil
		}
		if p.peek() == ';' {
			p.pos++
			continue
		}
		name, err := p.ident()
		if err != nil {
			return nil, err
		}
		if err := p.skip(); err != nil {
			return nil, err
		}
		if p.eof() || p.peek() != '=' {
			return nil, p.errf("atteso '=' dopo %q", name)
		}
		p.pos++
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out[name] = v
	}
}

type parser struct {
	src  []byte
	pos  int
	line int
}

func (p *parser) errf(format string, a ...any) error {
	return fmt.Errorf("svparse: riga %d: %s", p.line, fmt.Sprintf(format, a...))
}

func (p *parser) eof() bool  { return p.pos >= len(p.src) }
func (p *parser) peek() byte { return p.src[p.pos] }

// skip salta spazi e commenti (`-- ...` e `--[[ ... ]]`).
func (p *parser) skip() error {
	for !p.eof() {
		c := p.peek()
		switch {
		case c == '\n':
			p.line++
			p.pos++
		case c == ' ' || c == '\t' || c == '\r':
			p.pos++
		case c == '-' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '-':
			p.pos += 2
			if lvl, ok := p.longOpen(); ok {
				if _, err := p.longBody(lvl); err != nil {
					return err
				}
				continue
			}
			for !p.eof() && p.peek() != '\n' {
				p.pos++
			}
		default:
			return nil
		}
	}
	return nil
}

// longOpen riconosce `[[` o `[=*[` alla posizione corrente e consuma l'apertura.
func (p *parser) longOpen() (int, bool) {
	i := p.pos
	if i >= len(p.src) || p.src[i] != '[' {
		return 0, false
	}
	i++
	lvl := 0
	for i < len(p.src) && p.src[i] == '=' {
		lvl++
		i++
	}
	if i < len(p.src) && p.src[i] == '[' {
		p.pos = i + 1
		return lvl, true
	}
	return 0, false
}

func (p *parser) longBody(lvl int) (string, error) {
	closer := "]" + strings.Repeat("=", lvl) + "]"
	idx := bytes.Index(p.src[p.pos:], []byte(closer))
	if idx < 0 {
		p.line += bytes.Count(p.src[p.pos:], []byte("\n"))
		return "", p.errf("stringa lunga non chiusa")
	}
	body := p.src[p.pos : p.pos+idx]
	p.line += bytes.Count(body, []byte("\n"))
	p.pos += idx + len(closer)
	s := string(body)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "\r"), "\n")
	return s, nil
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func (p *parser) ident() (string, error) {
	if p.eof() || !isIdentStart(p.peek()) {
		return "", p.errf("carattere inatteso %s", p.describe())
	}
	start := p.pos
	for !p.eof() && (isIdentStart(p.peek()) || isDigit(p.peek())) {
		p.pos++
	}
	return string(p.src[start:p.pos]), nil
}

func (p *parser) describe() string {
	if p.eof() {
		return "(fine del file)"
	}
	return strconv.QuoteRune(rune(p.peek()))
}

func (p *parser) value() (any, error) {
	if err := p.skip(); err != nil {
		return nil, err
	}
	if p.eof() {
		return nil, p.errf("valore mancante, file troncato")
	}
	c := p.peek()
	switch {
	case c == '{':
		p.pos++
		return p.table()
	case c == '"' || c == '\'':
		return p.quoted()
	case c == '[':
		lvl, ok := p.longOpen()
		if !ok {
			return nil, p.errf("carattere inatteso '['")
		}
		return p.longBody(lvl)
	case c == '-' || c == '+' || c == '.' || isDigit(c):
		return p.number()
	case isIdentStart(c):
		id, _ := p.ident()
		switch id {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "nil":
			return nil, nil
		}
		return nil, p.errf("identificatore inatteso %q", id)
	}
	return nil, p.errf("carattere inatteso %s", p.describe())
}

func (p *parser) number() (any, error) {
	start := p.pos
	if p.peek() == '-' || p.peek() == '+' {
		p.pos++
	}
	isFloat := false
	hex := false
	if p.pos+1 < len(p.src) && p.src[p.pos] == '0' && (p.src[p.pos+1] == 'x' || p.src[p.pos+1] == 'X') {
		hex = true
		p.pos += 2
	}
loop:
	for !p.eof() {
		c := p.peek()
		switch {
		case isDigit(c):
		case hex && ((c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')):
		case c == '.':
			isFloat = true
		case !hex && (c == 'e' || c == 'E'):
			isFloat = true
			if p.pos+1 < len(p.src) && (p.src[p.pos+1] == '-' || p.src[p.pos+1] == '+') {
				p.pos++
			}
		default:
			break loop
		}
		p.pos++
	}
	txt := string(p.src[start:p.pos])
	if !isFloat {
		if n, err := strconv.ParseInt(txt, 0, 64); err == nil {
			return n, nil
		}
	}
	f, err := strconv.ParseFloat(txt, 64)
	if err != nil {
		return nil, p.errf("numero non valido %q", txt)
	}
	return f, nil
}

func (p *parser) quoted() (string, error) {
	q := p.peek()
	p.pos++
	var sb strings.Builder
	for {
		if p.eof() {
			return "", p.errf("stringa non chiusa")
		}
		c := p.peek()
		p.pos++
		switch c {
		case q:
			return sb.String(), nil
		case '\n':
			p.line++
			return "", p.errf("stringa non chiusa")
		case '\\':
			if p.eof() {
				return "", p.errf("stringa non chiusa")
			}
			e := p.peek()
			p.pos++
			switch e {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case 'a':
				sb.WriteByte('\a')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'v':
				sb.WriteByte('\v')
			case '\n':
				p.line++
				sb.WriteByte('\n')
			case 'x':
				if p.pos+2 > len(p.src) {
					return "", p.errf("escape \\x non valido")
				}
				n, err := strconv.ParseUint(string(p.src[p.pos:p.pos+2]), 16, 8)
				if err != nil {
					return "", p.errf("escape \\x non valido")
				}
				p.pos += 2
				sb.WriteByte(byte(n))
			default:
				if isDigit(e) {
					n := int(e - '0')
					for i := 0; i < 2 && !p.eof() && isDigit(p.peek()); i++ {
						n = n*10 + int(p.peek()-'0')
						p.pos++
					}
					if n > 255 {
						return "", p.errf("escape decimale fuori intervallo")
					}
					sb.WriteByte(byte(n))
				} else {
					sb.WriteByte(e) // \" \' \\ e altri
				}
			}
		default:
			sb.WriteByte(c)
		}
	}
}

// table legge il corpo dopo '{' fino alla '}'.
func (p *parser) table() (any, error) {
	m := map[string]any{}
	n := 0 // elementi posizionali
	arrayLike := true
	var maxKey int64
	ints := map[int64]bool{}
	setInt := func(i int64, v any) {
		m[strconv.FormatInt(i, 10)] = v
		if i >= 1 {
			ints[i] = true
			if i > maxKey {
				maxKey = i
			}
		} else {
			arrayLike = false
		}
	}
	for {
		if err := p.skip(); err != nil {
			return nil, err
		}
		if p.eof() {
			return nil, p.errf("tabella non chiusa, file troncato")
		}
		c := p.peek()
		if c == '}' {
			p.pos++
			break
		}
		if c == ',' || c == ';' {
			p.pos++
			continue
		}
		var key any // string, int64 o float64; nil = posizionale
		switch {
		case c == '[' && !p.isLongOpenAhead():
			p.pos++
			k, err := p.value()
			if err != nil {
				return nil, err
			}
			if err := p.skip(); err != nil {
				return nil, err
			}
			if p.eof() || p.peek() != ']' {
				return nil, p.errf("atteso ']'")
			}
			p.pos++
			key = k
		case isIdentStart(c) && p.isNamedField():
			id, _ := p.ident()
			key = id
		}
		if key != nil {
			if err := p.skip(); err != nil {
				return nil, err
			}
			if p.eof() || p.peek() != '=' {
				return nil, p.errf("atteso '='")
			}
			p.pos++
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		if key == nil {
			n++
			key = int64(n)
		}
		if v == nil {
			continue // nil = chiave assente
		}
		switch k := key.(type) {
		case int64:
			setInt(k, v)
		case float64:
			if k == float64(int64(k)) {
				setInt(int64(k), v)
			} else {
				m[strconv.FormatFloat(k, 'g', -1, 64)] = v
				arrayLike = false
			}
		case string:
			m[k] = v
			arrayLike = false
		default:
			return nil, p.errf("chiave di tabella non supportata")
		}
	}
	if arrayLike && int64(len(ints)) == maxKey && len(m) == len(ints) {
		arr := make([]any, maxKey)
		for i := int64(1); i <= maxKey; i++ {
			arr[i-1] = m[strconv.FormatInt(i, 10)]
		}
		return arr, nil
	}
	return m, nil
}

// isLongOpenAhead: '[' seguito da '[' o '=' e' una stringa lunga, non una chiave.
func (p *parser) isLongOpenAhead() bool {
	save := p.pos
	_, ok := p.longOpen()
	p.pos = save
	return ok
}

// isNamedField: identificatore seguito (dopo spazi) da '=' ma non '=='.
func (p *parser) isNamedField() bool {
	i := p.pos
	for i < len(p.src) && (isIdentStart(p.src[i]) || isDigit(p.src[i])) {
		i++
	}
	for i < len(p.src) && (p.src[i] == ' ' || p.src[i] == '\t' || p.src[i] == '\r' || p.src[i] == '\n') {
		i++
	}
	return i < len(p.src) && p.src[i] == '=' && (i+1 >= len(p.src) || p.src[i+1] != '=')
}
