package combatlog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var rome = time.FixedZone("CEST", 2*3600)

const (
	gino  = `Player-1234-0A1B2C3D,"Gino-Realm",0x511,0x0`
	murl  = `Creature-0-5250-0-17-46-00003A1B01,"Murloc Forager",0xa48,0x0`
	adv17 = `Creature-0-5250-0-17-46-00003A1B01,0000000000000000,420,420,0,0,0,0,1,0,0,0,-9450.12,62.34,1429,1.5708,10`
	adv19 = adv17 + `,63,0`
	dmg10 = `58,-1,1,0,0,0,%s,nil,nil,nil`
	dmg11 = `58,58,-1,1,0,0,0,%s,nil,nil,nil`
)

func line(sub string, parts ...string) string {
	return "10/24/2026 21:02:03.4410+2  " + sub + "," + strings.Join(parts, ",")
}

func crit(tmpl, c string) string { return strings.Replace(tmpl, "%s", c, 1) }

func TestParseLineSwingVariants(t *testing.T) {
	want := time.Date(2026, 10, 24, 21, 2, 3, 441_000_000, rome)
	cases := []struct {
		name string
		line string
		crit bool
	}{
		{"senza avanzati, 10", line("SWING_DAMAGE", murl, gino, crit(dmg10, "nil")), false},
		{"senza avanzati, 11", line("SWING_DAMAGE", murl, gino, crit(dmg11, "1")), true},
		{"avanzati 17, 10", line("SWING_DAMAGE", murl, gino, adv17, crit(dmg10, "1")), true},
		{"avanzati 17, 11", line("SWING_DAMAGE", murl, gino, adv17, crit(dmg11, "nil")), false},
		{"avanzati 19, 10", line("SWING_DAMAGE", murl, gino, adv19, crit(dmg10, "nil")), false},
		{"avanzati 19, 11", line("SWING_DAMAGE", murl, gino, adv19, crit(dmg11, "1")), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev, ok := ParseLine(c.line, rome, 0)
			if !ok {
				t.Fatal("ok = false")
			}
			if ev.Amount != 58 || ev.Crit != c.crit {
				t.Fatalf("amount %d crit %v", ev.Amount, ev.Crit)
			}
			if !ev.Time.Equal(want) {
				t.Fatalf("time %v", ev.Time)
			}
			if ev.Sub != "SWING_DAMAGE" || ev.SrcName != "Murloc Forager" || ev.DstName != "Gino-Realm" ||
				ev.SrcFlags != 0xa48 || ev.DstFlags != 0x511 ||
				ev.SrcGUID != "Creature-0-5250-0-17-46-00003A1B01" || ev.DstGUID != "Player-1234-0A1B2C3D" {
				t.Fatalf("base %+v", ev)
			}
		})
	}
}

func TestParseLineSpellAndHeal(t *testing.T) {
	spell := `78,"Heroic Strike",0x1`
	heal4 := `640,0,0,%s`
	heal5 := `640,640,0,0,%s`
	cases := []struct {
		name, line, sub, spell string
		amount                 int64
		crit                   bool
	}{
		{"spell 17/11", line("SPELL_DAMAGE", gino, murl, spell, adv17, crit(dmg11, "1")), "SPELL_DAMAGE", "Heroic Strike", 58, true},
		{"spell 19/10", line("SPELL_DAMAGE", gino, murl, spell, adv19, crit(dmg10, "nil")), "SPELL_DAMAGE", "Heroic Strike", 58, false},
		{"spell senza avanzati", line("RANGE_DAMAGE", gino, murl, spell, crit(dmg10, "1")), "RANGE_DAMAGE", "Heroic Strike", 58, true},
		{"periodico", line("SPELL_PERIODIC_DAMAGE", gino, murl, spell, adv17, crit(dmg11, "nil")), "SPELL_PERIODIC_DAMAGE", "Heroic Strike", 58, false},
		{"cura 17/4", line("SPELL_HEAL", gino, gino, `2061,"Flash Heal",0x2`, adv17, crit(heal4, "1")), "SPELL_HEAL", "Flash Heal", 640, true},
		{"cura 17/5", line("SPELL_HEAL", gino, gino, `2061,"Flash Heal",0x2`, adv17, crit(heal5, "nil")), "SPELL_HEAL", "Flash Heal", 640, false},
		{"cura 19/5", line("SPELL_PERIODIC_HEAL", gino, gino, `139,"Renew",0x2`, adv19, crit(heal5, "1")), "SPELL_PERIODIC_HEAL", "Renew", 640, true},
		{"cura senza avanzati 5", line("SPELL_HEAL", gino, gino, `2061,"Flash Heal",0x2`, crit(heal5, "1")), "SPELL_HEAL", "Flash Heal", 640, true},
		{"aura (sconosciuto)", line("SPELL_AURA_APPLIED", gino, gino, `139,"Renew",0x2`, "BUFF"), "SPELL_AURA_APPLIED", "Renew", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev, ok := ParseLine(c.line, rome, 0)
			if !ok {
				t.Fatal("ok = false")
			}
			if ev.Sub != c.sub || ev.SpellName != c.spell || ev.Amount != c.amount || ev.Crit != c.crit {
				t.Fatalf("%+v", ev)
			}
		})
	}
}

func TestParseLineQuotedComma(t *testing.T) {
	l := line("SWING_DAMAGE", `Creature-0-1-0-1-99-0000000001,"Gnasher, the Old",0xa48,0x0`, gino, crit(dmg10, "nil"))
	ev, ok := ParseLine(l, rome, 0)
	if !ok || ev.SrcName != "Gnasher, the Old" || ev.Amount != 58 || ev.DstName != "Gino-Realm" {
		t.Fatalf("ok %v %+v", ok, ev)
	}
}

func TestParseLineEnvironmental(t *testing.T) {
	env := `0000000000000000,nil,0x80000000,0x80000000`
	advG := `Player-1234-0A1B2C3D,0000000000000000,1960,1960,312,0,640,0,1,0,100,0,-9451.00,61.90,1429,4.7124,12`
	for name, l := range map[string]string{
		"avanzati prima": line("ENVIRONMENTAL_DAMAGE", env, gino, advG, "Falling", `340,340,-1,1,0,0,0,nil,nil,nil,nil`),
		"tipo prima":     line("ENVIRONMENTAL_DAMAGE", env, gino, "Falling", advG, `340,-1,1,0,0,0,nil,nil,nil,nil`),
		"senza avanzati": line("ENVIRONMENTAL_DAMAGE", env, gino, "Falling", `340,340,-1,1,0,0,0,nil,nil,nil,nil`),
	} {
		t.Run(name, func(t *testing.T) {
			ev, ok := ParseLine(l, rome, 0)
			if !ok || ev.EnvType != "Falling" || ev.Amount != 340 || ev.SrcName != "" || ev.DstName != "Gino-Realm" {
				t.Fatalf("ok %v %+v", ok, ev)
			}
		})
	}
}

func TestParseLineTimeFormats(t *testing.T) {
	body := "  SWING_DAMAGE," + murl + "," + gino + "," + crit(dmg10, "nil")
	cases := []struct {
		prefix string
		year   int
		want   time.Time
	}{
		{"10/24/2026 21:02:03.4410+2", 0, time.Date(2026, 10, 24, 21, 2, 3, 441_000_000, rome)},
		{"10/24/2026 21:02:03.4410-4", 0, time.Date(2026, 10, 24, 21, 2, 3, 441_000_000, rome)}, // l'offset è ignorato: vale loc
		{"10/24/2026 21:02:03.4410", 0, time.Date(2026, 10, 24, 21, 2, 3, 441_000_000, rome)},
		{"10/24 21:02:03.441", 2025, time.Date(2025, 10, 24, 21, 2, 3, 441_000_000, rome)},
		{"1/5 9:02:03.5", 2027, time.Date(2027, 1, 5, 9, 2, 3, 500_000_000, rome)},
	}
	for _, c := range cases {
		ev, ok := ParseLine(c.prefix+body, rome, c.year)
		if !ok || !ev.Time.Equal(c.want) {
			t.Fatalf("%q: ok %v time %v", c.prefix, ok, ev.Time)
		}
	}
}

func TestParseLineSpecialAndMalformed(t *testing.T) {
	for _, l := range []string{
		`10/24/2026 21:01:58.1020+2  COMBAT_LOG_VERSION,22,ADVANCED_LOG_ENABLED,1,BUILD_VERSION,11.2.5,PROJECT_ID,1`,
		`10/24/2026 21:01:58.1020+2  ZONE_CHANGE,1429,"Elwynn Forest",0`,
		`10/24/2026 21:01:58.1020+2  MAP_CHANGE,1429,"Elwynn Forest",-7939.58,-10254.2,-1379.17,1725.0`,
		`10/24/2026 21:01:58.1020+2  ENCOUNTER_START,1052,"Hogger",1,5,34`,
		`10/24/2026 21:01:58.1020+2  COMBATANT_INFO,Player-1234-0A1B2C3D,1,100,200,[(1,2,3),(4,5)],"x,y",(1,2)`,
	} {
		ev, ok := ParseLine(l, rome, 0)
		if !ok || ev.Sub == "" || ev.SrcGUID != "" || ev.DstGUID != "" || ev.Time.IsZero() {
			t.Fatalf("%q: ok %v %+v", l, ok, ev)
		}
	}
	for _, l := range []string{
		"",
		"   ",
		"ciao",
		"10/24/2026 21:02:03.4410+2 SWING_DAMAGE,a",       // uno spazio solo
		"10/24/2026 21:02:03.4410+2  ",                    // niente sottoevento
		"13/45/2026 21:02:03.4410+2  ZONE_CHANGE,1,\"x\"", // data impossibile
		"10/24/2026 25:02:03.4410+2  ZONE_CHANGE,1,\"x\"",
		"aa/bb 21:02:03.4  ZONE_CHANGE,1",
		line("SWING_DAMAGE", murl, `Player-1,"Gino`),           // virgolette non chiuse
		line("SWING_DAMAGE", murl, `Player-1,"Gino",0x511`),    // base incompleta
		line("SWING_DAMAGE", murl, `Player-1,"Gino",zz,0x0,1`), // flag non esadecimale
	} {
		if ev, ok := ParseLine(l, rome, 2026); ok {
			t.Fatalf("%q: atteso ok=false, %+v", l, ev)
		}
	}
}

func TestParseLineNoPanic(t *testing.T) {
	base := line("SPELL_DAMAGE", gino, murl, `78,"Heroic Strike",0x1`, adv17, crit(dmg11, "1"))
	for i := 0; i <= len(base); i++ {
		ParseLine(base[:i], rome, 2026)
	}
	for _, sub := range []string{"SWING_DAMAGE", "SPELL_HEAL", "ENVIRONMENTAL_DAMAGE", "SPELL_DAMAGE", "DAMAGE_SPLIT"} {
		for n := 0; n < 40; n++ {
			parts := []string{murl, gino}
			for k := 0; k < n; k++ {
				parts = append(parts, "Player-1-2")
			}
			ParseLine(line(sub, parts...), rome, 2026)
		}
	}
}

func copyMurloc(t *testing.T, dir, name string, mod time.Time) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "murloc.txt"))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMurlocFixture(t *testing.T) {
	dir := t.TempDir()
	p := copyMurloc(t, dir, "WoWCombatLog-102426_210158.txt", time.Date(2026, 10, 24, 23, 0, 0, 0, rome))
	evs, err := Read([]string{p}, time.Time{}, time.Time{}, rome)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 38 {
		t.Fatalf("eventi %d", len(evs))
	}
	creatures := map[string]bool{}
	var healed, falling, died int
	for _, e := range evs {
		if e.Time.Before(time.Date(2026, 10, 24, 21, 0, 0, 0, rome)) || e.Time.After(time.Date(2026, 10, 24, 21, 10, 0, 0, rome)) {
			t.Fatalf("ora fuori serata: %v", e.Time)
		}
		if e.Sub == "SWING_DAMAGE" && e.DstName == "Gino-Realm" {
			if e.SrcFlags&0x40 == 0 || e.Amount <= 0 {
				t.Fatalf("%+v", e)
			}
			creatures[e.SrcGUID] = true
		}
		if e.Sub == "SPELL_HEAL" && e.SrcName == "Mara-Realm" && e.Amount > 0 {
			healed++
		}
		if e.EnvType == "Falling" && e.Amount == 340 {
			falling++
		}
		if e.Sub == "UNIT_DIED" {
			died++
		}
	}
	if len(creatures) != 6 || healed != 3 || falling != 1 || died != 1 {
		t.Fatalf("creature %d cure %d cadute %d morti %d", len(creatures), healed, falling, died)
	}
}

func TestReadWindow(t *testing.T) {
	dir := t.TempDir()
	p := copyMurloc(t, dir, "WoWCombatLog.txt", time.Date(2026, 10, 24, 23, 0, 0, 0, rome))
	// esclude le 3 righe di intestazione e il primo colpo; include fino alla fine del combattimento.
	from := time.Date(2026, 10, 24, 21, 2, 4, 0, rome)
	to := time.Date(2026, 10, 24, 21, 2, 21, 500_000_000, rome) // inclusa
	evs, err := Read([]string{p}, from, to, rome)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 31 {
		t.Fatalf("eventi %d", len(evs))
	}
	if evs[0].Sub != "SWING_DAMAGE" || evs[0].SrcName != "Gino-Realm" || evs[len(evs)-1].SrcName != "Murloc Netter" {
		t.Fatalf("primo %+v ultimo %+v", evs[0], evs[len(evs)-1])
	}
}

func TestReadSkipsOldFilesAndErrors(t *testing.T) {
	dir := t.TempDir()
	old := copyMurloc(t, dir, "WoWCombatLog-old.txt", time.Date(2026, 10, 20, 23, 0, 0, 0, rome))
	evs, err := Read([]string{old}, time.Date(2026, 10, 24, 0, 0, 0, 0, rome), time.Time{}, rome)
	if err != nil || len(evs) != 0 {
		t.Fatalf("err %v eventi %d", err, len(evs))
	}
	if _, err := Read([]string{filepath.Join(dir, "manca.txt")}, time.Time{}, time.Time{}, rome); err == nil {
		t.Fatal("atteso errore per file mancante")
	}
}

func TestReadLongLinesAndCRLF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "WoWCombatLog.txt")
	long := "10/24/2026 21:02:00.0000+2  COMBATANT_INFO," + strings.Repeat("x", 3<<20)
	ok1 := line("SWING_DAMAGE", murl, gino, crit(dmg10, "nil"))
	content := bom + ok1 + "\r\n" + long + "\r\n" + "rumore\r\n\r\n" + ok1 // ultima riga senza a capo
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	evs, err := Read([]string{p}, time.Time{}, time.Time{}, rome)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[0].Amount != 58 || evs[1].Amount != 58 {
		t.Fatalf("eventi %d %+v", len(evs), evs)
	}
}

func TestFiles(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"WoWCombatLog-102426_210158.txt", "WoWCombatLog.txt", "WoWCombatLog-102326_200000.txt", "altro.txt", "WoWCombatLog.zip"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "WoWCombatLog-dir.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := Files(dir)
	want := []string{
		filepath.Join(dir, "WoWCombatLog-102326_200000.txt"),
		filepath.Join(dir, "WoWCombatLog-102426_210158.txt"),
		filepath.Join(dir, "WoWCombatLog.txt"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if got := Files(filepath.Join(dir, "manca")); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	// cartella con parentesi quadre nel nome (non deve essere trattata come glob)
	br := filepath.Join(dir, "[x]")
	if err := os.Mkdir(br, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(br, "WoWCombatLog.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Files(br); len(got) != 1 {
		t.Fatalf("%v", got)
	}
}

func TestReadYearlessLines(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "WoWCombatLog.txt")
	content := "12/31 23:59:00.000  ZONE_CHANGE,1429,\"Elwynn Forest\",0\n1/1 0:01:00.000  ZONE_CHANGE,1429,\"Elwynn Forest\",0\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	mod := time.Date(2027, 1, 1, 0, 5, 0, 0, rome)
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
	evs, err := Read([]string{p}, time.Time{}, time.Time{}, rome)
	if err != nil || len(evs) != 2 {
		t.Fatalf("err %v eventi %d", err, len(evs))
	}
	if evs[0].Time.Year() != 2026 || evs[1].Time.Year() != 2027 {
		t.Fatalf("%v %v", evs[0].Time, evs[1].Time)
	}
}
