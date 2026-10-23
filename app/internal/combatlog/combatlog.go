// Package combatlog legge i file di testo WoWCombatLog*.txt del client.
//
// Formato di una riga: `M/D[/YYYY] H:MM:SS.fff[±tz]  SUB,campi…` (due spazi tra
// data/ora e CSV). L'eventuale offset di fuso dopo l'ora (es. `-4`, `+2`) viene
// ignorato: l'ora è interpretata nel fuso loc passato dal chiamante, come per il
// formato vecchio che non ha l'offset.
//
// Registro avanzato e suffissi: il blocco avanzato (17 campi, 19 in alcune
// versioni) si riconosce dal primo campo con forma di GUID. La sua lunghezza si
// deduce contando dalla fine della riga: il suffisso _DAMAGE ha 10 campi (11 con
// baseAmount), il suffisso _HEAL 4 (5 con baseAmount). Si sceglie la combinazione
// avanzato+suffisso che torna con il numero di campi; se nessuna torna si assume
// il blocco da 17 e il suffisso è quello che resta. `critical` è letto dalla sua
// posizione nel suffisso (dopo baseAmount se presente).
package combatlog

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Event è una riga del registro. I campi non presenti nella riga restano vuoti.
type Event struct {
	Time      time.Time
	Sub       string
	SrcGUID   string
	SrcName   string
	SrcFlags  uint32
	DstGUID   string
	DstName   string
	DstFlags  uint32
	SpellName string
	Amount    int64
	Crit      bool
	EnvType   string
}

// MaxLine è la lunghezza massima di una riga; le righe più lunghe vengono saltate.
const MaxLine = 1 << 20

var advLens = []int{17, 19}

var bom = string(rune(0xFEFF))

// combatPrefixes: sottoeventi che devono avere il prefisso base di 8 campi.
var combatPrefixes = []string{"SWING_", "RANGE_", "SPELL_", "ENVIRONMENTAL_", "DAMAGE_", "UNIT_", "PARTY_KILL", "ENCHANT_"}

var guidPrefixes = []string{"Player-", "Creature-", "Pet-", "Vehicle-", "GameObject-", "Vignette-", "Item-", "BattlePet-", "Cast-", "ClientActor-"}

func isGUID(s string) bool {
	if s == "0000000000000000" {
		return true
	}
	for _, p := range guidPrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// ParseLine interpreta una riga. year si usa quando la data non ha l'anno
// (0 = anno corrente). ok è false per righe vuote o malformate.
func ParseLine(line string, loc *time.Location, year int) (Event, bool) {
	stamp, body, ok := splitLine(line)
	if !ok {
		return Event{}, false
	}
	t, ok := parseTime(stamp, loc, year)
	if !ok {
		return Event{}, false
	}
	return parseBody(t, body)
}

func splitLine(line string) (stamp, body string, ok bool) {
	line = strings.TrimPrefix(line, bom)
	line = strings.TrimRight(line, "\r\n")
	i := strings.Index(line, "  ")
	if i <= 0 {
		return "", "", false
	}
	return line[:i], line[i+2:], true
}

func atoiRange(s string, lo, hi int) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < lo || n > hi {
		return 0, false
	}
	return n, true
}

func parseTime(stamp string, loc *time.Location, year int) (time.Time, bool) {
	if loc == nil {
		loc = time.Local
	}
	date, clock, ok := strings.Cut(stamp, " ")
	if !ok {
		return time.Time{}, false
	}
	dp := strings.Split(date, "/")
	if len(dp) != 2 && len(dp) != 3 {
		return time.Time{}, false
	}
	mon, ok1 := atoiRange(dp[0], 1, 12)
	day, ok2 := atoiRange(dp[1], 1, 31)
	if !ok1 || !ok2 {
		return time.Time{}, false
	}
	if len(dp) == 3 {
		y, ok := atoiRange(dp[2], 1, 9999)
		if !ok {
			return time.Time{}, false
		}
		year = y
	} else if year <= 0 {
		year = time.Now().In(loc).Year()
	}
	// offset di fuso finale (+2, -4, …): ignorato, vale loc.
	if i := strings.IndexAny(clock, "+-"); i >= 0 {
		clock = clock[:i]
	}
	hms, frac, _ := strings.Cut(clock, ".")
	cp := strings.Split(hms, ":")
	if len(cp) != 3 {
		return time.Time{}, false
	}
	h, ok1 := atoiRange(cp[0], 0, 23)
	m, ok2 := atoiRange(cp[1], 0, 59)
	s, ok3 := atoiRange(cp[2], 0, 60)
	if !ok1 || !ok2 || !ok3 {
		return time.Time{}, false
	}
	ns := 0
	if frac != "" {
		if len(frac) > 9 {
			frac = frac[:9]
		}
		f, err := strconv.Atoi(frac)
		if err != nil || f < 0 {
			return time.Time{}, false
		}
		for k := len(frac); k < 9; k++ {
			f *= 10
		}
		ns = f
	}
	t := time.Date(year, time.Month(mon), day, h, m, s, ns, loc)
	if t.Day() != day || int(t.Month()) != mon {
		return time.Time{}, false // es. 31/2
	}
	return t, true
}

// splitCSV divide sulle virgole fuori dalle virgolette; i campi tra virgolette
// perdono le virgolette, `nil` non quotato diventa "".
func splitCSV(s string) ([]string, bool) {
	var out []string
	for {
		var field string
		if strings.HasPrefix(s, `"`) {
			end := strings.IndexByte(s[1:], '"')
			if end < 0 {
				return nil, false
			}
			field = s[1 : 1+end]
			s = s[2+end:]
			if s != "" && s[0] != ',' {
				// testo dopo la virgoletta di chiusura: lo si accoda al campo
				j := strings.IndexByte(s, ',')
				if j < 0 {
					j = len(s)
				}
				field += s[:j]
				s = s[j:]
			}
		} else {
			j := strings.IndexByte(s, ',')
			if j < 0 {
				j = len(s)
			}
			field = strings.TrimSpace(s[:j])
			if field == "nil" {
				field = ""
			}
			s = s[j:]
		}
		out = append(out, field)
		if s == "" {
			return out, true
		}
		s = s[1:] // virgola
	}
}

func hasAnyPrefix(s string, ps []string) bool {
	for _, p := range ps {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func parseFlags(s string) (uint32, bool) {
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return 0, false
	}
	v, err := strconv.ParseUint(s[2:], 16, 32)
	return uint32(v), err == nil
}

func parseBody(t time.Time, body string) (Event, bool) {
	sub, rest, _ := strings.Cut(body, ",")
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return Event{}, false
	}
	ev := Event{Time: t, Sub: sub}
	combat := hasAnyPrefix(sub, combatPrefixes)
	f, ok := splitCSV(rest)
	if !ok {
		if combat {
			return Event{}, false
		}
		return ev, true // riga speciale con formato inatteso: basta ora e tipo
	}
	if !fillBase(&ev, f) {
		if combat {
			return Event{}, false
		}
		return ev, true // COMBAT_LOG_VERSION, ZONE_CHANGE, ENCOUNTER_START, COMBATANT_INFO, …
	}
	if !combat {
		return ev, true
	}
	f = f[8:]
	switch {
	case strings.HasPrefix(sub, "SPELL_"), strings.HasPrefix(sub, "RANGE_"),
		sub == "DAMAGE_SHIELD", sub == "DAMAGE_SPLIT", sub == "DAMAGE_SHIELD_MISSED":
		if len(f) < 3 {
			return ev, true
		}
		ev.SpellName = f[1]
		f = f[3:]
	case strings.HasPrefix(sub, "ENVIRONMENTAL_"):
		if len(f) == 0 {
			return ev, true
		}
		if !isGUID(f[0]) {
			// tipo prima del blocco avanzato
			ev.EnvType = f[0]
			f = f[1:]
		} else {
			// blocco avanzato prima del tipo: si individua contando dalla fine
			adv, suf := layout(len(f)-1, damageLens(sub))
			if adv+1+suf > len(f) || adv >= len(f) {
				return ev, true
			}
			ev.EnvType = f[adv]
			fillSuffix(&ev, sub, f[adv+1:])
			return ev, true
		}
	}
	lens := damageLens(sub)
	if lens == nil {
		return ev, true
	}
	adv := 0
	if len(f) > 0 && isGUID(f[0]) {
		adv, _ = layout(len(f), lens)
	}
	if adv >= len(f) {
		return ev, true
	}
	fillSuffix(&ev, sub, f[adv:])
	return ev, true
}

func fillBase(ev *Event, f []string) bool {
	if len(f) < 8 {
		return false
	}
	sf, ok1 := parseFlags(f[2])
	df, ok2 := parseFlags(f[6])
	if !ok1 || !ok2 {
		return false
	}
	ev.SrcGUID, ev.SrcName, ev.SrcFlags = f[0], f[1], sf
	ev.DstGUID, ev.DstName, ev.DstFlags = f[4], f[5], df
	return true
}

func isDamage(sub string) bool {
	return strings.HasSuffix(sub, "_DAMAGE") || strings.HasSuffix(sub, "_DAMAGE_LANDED") ||
		sub == "DAMAGE_SHIELD" || sub == "DAMAGE_SPLIT"
}

// damageLens restituisce le lunghezze note del suffisso (senza e con baseAmount).
func damageLens(sub string) []int {
	switch {
	case isDamage(sub):
		return []int{10, 11}
	case strings.HasSuffix(sub, "_HEAL"):
		return []int{4, 5}
	}
	return nil
}

// layout sceglie la lunghezza del blocco avanzato che, con un suffisso di
// lunghezza nota, copre esattamente n campi. Altrimenti 17 e suffisso = resto.
func layout(n int, sufLens []int) (adv, suf int) {
	for _, a := range advLens {
		for _, s := range sufLens {
			if a+s == n {
				return a, s
			}
		}
	}
	if n-advLens[0] < 0 {
		return advLens[0], 0
	}
	return advLens[0], n - advLens[0]
}

func fillSuffix(ev *Event, sub string, s []string) {
	if len(s) == 0 {
		return
	}
	if v, err := strconv.ParseInt(s[0], 10, 64); err == nil {
		ev.Amount = v
	}
	critAt := -1
	switch {
	case isDamage(sub):
		critAt = 6 // amount, overkill, school, resisted, blocked, absorbed, critical
		if len(s) >= 11 {
			critAt = 7 // con baseAmount dopo amount
		}
	case strings.HasSuffix(sub, "_HEAL"):
		critAt = 3 // amount, overhealing, absorbed, critical
		if len(s) >= 5 {
			critAt = 4
		}
	}
	if critAt >= 0 && critAt < len(s) {
		ev.Crit = s[critAt] == "1"
	}
}

// Files elenca i file WoWCombatLog*.txt della cartella, in ordine di nome.
// Una cartella mancante dà un elenco vuoto.
func Files(logsDir string) []string {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, e := range entries {
		n := e.Name()
		l := strings.ToLower(n)
		if e.IsDir() || !strings.HasPrefix(l, "wowcombatlog") || !strings.HasSuffix(l, ".txt") {
			continue
		}
		out = append(out, filepath.Join(logsDir, n))
	}
	sort.Strings(out)
	return out
}

// Read legge i file in streaming e restituisce gli eventi con ora in [from, to].
// from zero = nessun limite inferiore, to zero = nessun limite superiore.
// I file modificati prima di from vengono saltati senza aprirli. Le righe
// malformate o più lunghe di MaxLine vengono ignorate.
func Read(paths []string, from, to time.Time, loc *time.Location) ([]Event, error) {
	var out []Event
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return out, fmt.Errorf("registro combattimenti %s: %w", p, err)
		}
		if !from.IsZero() && st.ModTime().Before(from) {
			continue
		}
		out, err = readFile(p, st.ModTime(), from, to, loc, out)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func readFile(p string, mod, from, to time.Time, loc *time.Location, out []Event) ([]Event, error) {
	fh, err := os.Open(p)
	if err != nil {
		return out, fmt.Errorf("registro combattimenti %s: %w", p, err)
	}
	defer fh.Close()
	if loc == nil {
		loc = time.Local
	}
	year := mod.In(loc).Year() // per le righe senza anno
	r := bufio.NewReaderSize(fh, 64<<10)
	var buf []byte
	tooLong := false
	for {
		chunk, isPrefix, err := r.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, fmt.Errorf("registro combattimenti %s: %w", p, err)
		}
		if !tooLong {
			if len(buf)+len(chunk) > MaxLine {
				tooLong, buf = true, buf[:0]
			} else {
				buf = append(buf, chunk...)
			}
		}
		if isPrefix {
			continue
		}
		if !tooLong {
			if ev, ok := parseInWindow(string(buf), loc, year, mod, from, to); ok {
				out = append(out, ev)
			}
		}
		buf, tooLong = buf[:0], false
	}
}

// parseInWindow legge prima solo l'ora, così le righe fuori finestra costano poco.
func parseInWindow(line string, loc *time.Location, year int, mod, from, to time.Time) (Event, bool) {
	stamp, body, ok := splitLine(line)
	if !ok {
		return Event{}, false
	}
	t, ok := parseTime(stamp, loc, year)
	if !ok {
		return Event{}, false
	}
	// riga senza anno scritta a dicembre e file modificato a gennaio: anno precedente.
	date, _, _ := strings.Cut(stamp, " ")
	if strings.Count(date, "/") == 1 && t.After(mod.Add(48*time.Hour)) {
		t = t.AddDate(-1, 0, 0)
	}
	if (!from.IsZero() && t.Before(from)) || (!to.IsZero() && t.After(to)) {
		return Event{}, false
	}
	return parseBody(t, body)
}
