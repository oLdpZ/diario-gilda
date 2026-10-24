// Package combatlog legge i file di testo WoWCombatLog*.txt del client.
//
// Formato di una riga: `M/D[/YYYY] H:MM:SS.fff[±tz]  SUB,campi…` (due spazi tra
// data/ora e CSV). L'eventuale offset di fuso dopo l'ora (es. `-4`, `+2`) viene
// ignorato: l'ora è interpretata nel fuso loc passato dal chiamante, come per il
// formato vecchio che non ha l'offset.
//
// Registro avanzato e suffissi: il blocco avanzato (17 campi, 19 in alcune
// versioni) si riconosce dal primo campo con forma di GUID e si misura dal
// davanti, dalla struttura: posX, posY, facing decimali e uiMapID intero (vedi
// advFront). Poi si controlla che dopo il blocco il suffisso abbia la forma
// attesa (interi, poi i flag "1"/nil; vedi suffixCrit), che dice anche se c'è
// baseAmount dopo amount. I campi in più alla fine della riga (es. `ST`/`AOE`
// dei client recenti) sono ignorati. Se nulla torna si assume il blocco da 17.
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
			// blocco avanzato prima del tipo
			adv := pickAdv(f, sub, 1)
			if adv >= len(f) {
				return ev, true
			}
			ev.EnvType = f[adv]
			fillSuffix(&ev, sub, f[adv+1:])
			return ev, true
		}
	}
	if !isDamage(sub) && !isHeal(sub) {
		return ev, true
	}
	adv := 0
	if len(f) > 0 && isGUID(f[0]) {
		adv = pickAdv(f, sub, 0)
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

func isHeal(sub string) bool { return strings.HasSuffix(sub, "_HEAL") }

func isInt(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func isDec(s string) bool {
	if !strings.Contains(s, ".") {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// isFlag: critical, glancing, crushing, isOffHand valgono "1" o nil ("").
func isFlag(s string) bool { return s == "" || s == "1" }

func all(s []string, from, to int, ok func(string) bool) bool {
	if to > len(s) {
		return false
	}
	for _, v := range s[from:to] {
		if !ok(v) {
			return false
		}
	}
	return true
}

// advFront riconosce la lunghezza del blocco avanzato dalla sua struttura: nel
// blocco da 17 i campi 12, 13, 15 (posX, posY, facing) sono decimali con il punto
// e il 14 (uiMapID) è intero; il blocco da 19 è lo stesso schema spostato di 2.
// 0 = schema non riconosciuto.
func advFront(f []string) int {
	for _, n := range advLens {
		p := n - 5
		if p+3 < len(f) && isDec(f[p]) && isDec(f[p+1]) && isInt(f[p+2]) && isDec(f[p+3]) {
			return n
		}
	}
	return 0
}

// pickAdv sceglie la lunghezza del blocco avanzato (che inizia in f[0]); skip è il
// numero di campi tra il blocco e il suffisso (1 per il tipo ambientale). Prima la
// lunghezza indicata dalla struttura del blocco, poi l'altra; vince la prima dopo
// la quale il suffisso ha la forma attesa. Se nessuna va bene: quella della
// struttura, o 17.
func pickAdv(f []string, sub string, skip int) int {
	cands := []int{17, 19}
	if advFront(f) == 19 {
		cands = []int{19, 17}
	}
	for _, c := range cands {
		if c+skip < len(f) && suffixCrit(sub, f[c+skip:]) >= 0 {
			return c
		}
	}
	return cands[0]
}

// suffixCrit restituisce la posizione di critical nel suffisso se questo ha la
// forma attesa, altrimenti -1. Danni: amount, [baseAmount], overkill, school,
// resisted, blocked, absorbed (interi) poi critical, glancing, crushing,
// isOffHand ("1"/nil). Cure: amount, [baseAmount], overhealing, absorbed (interi)
// poi critical. Si prova prima la forma con baseAmount; i campi in più alla fine
// (es. `ST`/`AOE`) sono ignorati.
func suffixCrit(sub string, s []string) int {
	switch {
	case isDamage(sub):
		for _, n := range []int{7, 6} {
			if all(s, 0, n, isInt) && all(s, n, n+4, isFlag) {
				return n
			}
		}
	case isHeal(sub):
		for _, n := range []int{4, 3} {
			if all(s, 0, n, isInt) && all(s, n, n+1, isFlag) {
				return n
			}
		}
	}
	return -1
}

func fillSuffix(ev *Event, sub string, s []string) {
	if len(s) == 0 {
		return
	}
	if v, err := strconv.ParseInt(s[0], 10, 64); err == nil {
		ev.Amount = v
	}
	critAt := suffixCrit(sub, s)
	if critAt < 0 { // forma inattesa: posizione dalla lunghezza
		switch {
		case isDamage(sub):
			critAt = 6
			if len(s) >= 11 {
				critAt = 7
			}
		case isHeal(sub):
			critAt = 3
			if len(s) >= 5 {
				critAt = 4
			}
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

// Read legge i file in streaming e restituisce gli eventi con ora in [from, to],
// ordinati per ora (ordinamento stabile: a parità d'ora resta l'ordine del file).
// from zero = nessun limite inferiore, to zero = nessun limite superiore.
// I file modificati prima di from vengono saltati senza aprirli. Le righe
// malformate o più lunghe di MaxLine vengono ignorate, e così l'ultima riga del
// file se non termina con un a capo (il client potrebbe starla scrivendo).
// Un file che sparisce o non si riesce a leggere viene saltato: si continua con
// gli altri e alla fine si restituiscono gli eventi letti insieme agli errori
// uniti con errors.Join.
func Read(paths []string, from, to time.Time, loc *time.Location) ([]Event, error) {
	return ReadFiltered(paths, from, to, loc, nil)
}

// ReadFiltered e' Read che tiene solo gli eventi per cui keep restituisce true
// (nil = tutti); keep vede le righe valide nella finestra, nell'ordine dei file,
// cosi' in memoria resta solo quel che serve.
func ReadFiltered(paths []string, from, to time.Time, loc *time.Location, keep func(Event) bool) ([]Event, error) {
	var out []Event
	var errs []error
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			errs = append(errs, fmt.Errorf("registro combattimenti %s: %w", p, err))
			continue
		}
		if !from.IsZero() && st.ModTime().Before(from) {
			continue
		}
		out, err = readFile(p, st.ModTime(), from, to, loc, keep, out)
		if err != nil {
			errs = append(errs, err)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, errors.Join(errs...)
}

func readFile(p string, mod, from, to time.Time, loc *time.Location, keep func(Event) bool, out []Event) ([]Event, error) {
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
		chunk, err := r.ReadSlice('\n')
		if !tooLong {
			if len(buf)+len(chunk) > MaxLine+2 { // +2 per "\r\n"
				tooLong, buf = true, buf[:0]
			} else {
				buf = append(buf, chunk...)
			}
		}
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue // riga più lunga del buffer: si continua ad accumulare
		case errors.Is(err, io.EOF):
			return out, nil // l'eventuale riga senza a capo è incompleta: ignorata
		case err != nil:
			return out, fmt.Errorf("registro combattimenti %s: %w", p, err)
		}
		if !tooLong {
			if ev, ok := parseInWindow(string(buf), loc, year, mod, from, to); ok && (keep == nil || keep(ev)) {
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
