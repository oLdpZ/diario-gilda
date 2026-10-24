// Package episodes riassume gli eventi del registro combattimenti in "episodi"
// (uno per combattimento o morte) per il diario di gilda.
package episodes

import (
	"sort"
	"strings"
	"time"

	"github.com/oLdpZ/diario-gilda/app/internal/addondata"
	"github.com/oLdpZ/diario-gilda/app/internal/combatlog"
)

const hostileFlag = 0x40 // COMBATLOG_OBJECT_REACTION_HOSTILE

const maxHealers = 5

type Hit struct {
	Who    string `json:"who"`
	Spell  string `json:"spell"`
	Amount int64  `json:"amount"`
	Crit   bool   `json:"crit"`
}

type NameCount struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

type NameAmount struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}

type Episode struct {
	From            int64        `json:"from"`
	To              int64        `json:"to"`
	Kind            string       `json:"kind"` // "combat" | "death"
	MinHP           *int         `json:"minHp,omitempty"`
	EnemyCount      int          `json:"enemyCount"`
	Enemies         []NameCount  `json:"enemies"`
	DamageTaken     int64        `json:"damageTaken"`
	BiggestHitTaken *Hit         `json:"biggestHitTaken,omitempty"`
	BiggestHitDone  *Hit         `json:"biggestHitDone,omitempty"`
	KilledBy        *Hit         `json:"killedBy,omitempty"`
	Healers         []NameAmount `json:"healers"`
	Environmental   []NameAmount `json:"environmental"`
	Kills           []NameCount  `json:"kills"`
}

// Build crea gli episodi della sessione. Gli orari degli eventi della sessione
// sono secondi epoch; quelli del registro sono confrontati in secondi Unix
// (loc e' gia' stato applicato da combatlog). Finestre inclusive.
func Build(s addondata.Session, evs []combatlog.Event, loc *time.Location) []Episode {
	_ = loc
	b := builder{isPlayerName: PlayerMatcher(s), evs: evs}
	out := []Episode{}
	for _, e := range s.Events {
		switch e.K {
		case "combat":
			from, to := fieldInt(e.Fields, "from", e.T), fieldInt(e.Fields, "to", e.T)
			ep := b.episode(from-2, to+2)
			ep.From, ep.To, ep.Kind = from, to, "combat"
			if v, ok := fieldNum(e.Fields, "minHp"); ok {
				n := int(v)
				ep.MinHP = &n
			}
			out = append(out, ep)
		case "death":
			ep := b.episode(e.T-15, e.T)
			ep.From, ep.To, ep.Kind = e.T-15, e.T, "death"
			out = append(out, ep)
		}
	}
	return out
}

func fieldNum(f map[string]any, k string) (int64, bool) {
	switch n := f[k].(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	}
	return 0, false
}

func fieldInt(f map[string]any, k string, def int64) int64 {
	if n, ok := fieldNum(f, k); ok {
		return n
	}
	return def
}

type builder struct {
	isPlayerName func(string) bool
	evs          []combatlog.Event
}

// PlayerMatcher riconosce il nome del giocatore della sessione nel registro
// combattimenti: "Char" oppure "Char-Reame", con il reame confrontato senza
// spazi, apostrofi e trattini e senza badare alle maiuscole (il registro scrive
// "Gino-TarrenMill" per il reame "Tarren Mill"). Senza reame nella sessione
// vale qualsiasi "Char-…".
func PlayerMatcher(s addondata.Session) func(name string) bool {
	char, realm := s.Char, normRealm(s.Realm)
	return func(n string) bool {
		if char == "" {
			return false
		}
		if n == char {
			return true
		}
		rest, ok := strings.CutPrefix(n, char+"-")
		if !ok {
			return false
		}
		return realm == "" || normRealm(rest) == realm
	}
}

var realmPunct = strings.NewReplacer(" ", "", "'", "", "’", "", "-", "")

func normRealm(r string) string { return strings.ToLower(realmPunct.Replace(r)) }

// Relevant e' il filtro per combatlog.ReadFiltered: tiene solo le righe che
// servono a Build, cioe' quelle con il giocatore come sorgente o bersaglio (per
// nome o per il GUID imparato dalla prima riga con il suo nome) e le morti
// (UNIT_DIED non ha sorgente, serve per contare le uccisioni). Ha stato: un
// filtro per lettura.
func Relevant(s addondata.Session) func(combatlog.Event) bool {
	isName := PlayerMatcher(s)
	guid := ""
	return func(e combatlog.Event) bool {
		if e.Sub == "UNIT_DIED" {
			return true
		}
		for _, p := range [][2]string{{e.SrcName, e.SrcGUID}, {e.DstName, e.DstGUID}} {
			if isName(p[0]) {
				if guid == "" && strings.HasPrefix(p[1], "Player-") {
					guid = p[1]
				}
				return true
			}
			if guid != "" && p[1] == guid {
				return true
			}
		}
		return false
	}
}

func isDamage(sub string) bool { return strings.HasSuffix(sub, "_DAMAGE") }
func isHeal(sub string) bool   { return strings.HasSuffix(sub, "_HEAL") }

func spellOf(e combatlog.Event) string {
	switch {
	case e.SpellName != "":
		return e.SpellName
	case strings.HasPrefix(e.Sub, "SWING_"):
		return "Mischia"
	}
	return ""
}

func (b *builder) episode(lo, hi int64) Episode {
	ep := Episode{Enemies: []NameCount{}, Healers: []NameAmount{}, Environmental: []NameAmount{}, Kills: []NameCount{}}
	start := sort.Search(len(b.evs), func(i int) bool { return b.evs[i].Time.Unix() >= lo })
	var win []combatlog.Event
	for i := start; i < len(b.evs) && b.evs[i].Time.Unix() <= hi; i++ {
		win = append(win, b.evs[i])
	}

	// impara il GUID del giocatore dal primo evento con il suo nome
	playerGUID := ""
	for _, e := range win {
		if strings.HasPrefix(e.DstGUID, "Player-") && b.isPlayerName(e.DstName) {
			playerGUID = e.DstGUID
			break
		}
		if strings.HasPrefix(e.SrcGUID, "Player-") && b.isPlayerName(e.SrcName) {
			playerGUID = e.SrcGUID
			break
		}
	}
	isSrc := func(e combatlog.Event) bool {
		return b.isPlayerName(e.SrcName) || (playerGUID != "" && e.SrcGUID == playerGUID)
	}
	isDst := func(e combatlog.Event) bool {
		return b.isPlayerName(e.DstName) || (playerGUID != "" && e.DstGUID == playerGUID)
	}

	enemyGUIDs := map[string]string{} // guid -> nome
	hitTargets := map[string]bool{}   // GUID colpiti dal giocatore
	healers := map[string]int64{}
	env := map[string]int64{}
	kills := map[string]int{}
	var lastHit, taken, done *Hit

	for _, e := range win {
		switch {
		case isDamage(e.Sub) && isDst(e) && !isSrc(e):
			ep.DamageTaken += e.Amount
			if strings.HasPrefix(e.Sub, "ENVIRONMENTAL_") {
				name := e.EnvType
				if name == "" {
					name = e.SpellName
				}
				env[name] += e.Amount
				lastHit = &Hit{Who: name, Spell: name, Amount: e.Amount, Crit: e.Crit}
				continue
			}
			h := Hit{Who: e.SrcName, Spell: spellOf(e), Amount: e.Amount, Crit: e.Crit}
			lastHit = &h
			if taken == nil || h.Amount > taken.Amount {
				c := h
				taken = &c
			}
			if e.SrcFlags&hostileFlag != 0 && e.SrcGUID != "" {
				enemyGUIDs[e.SrcGUID] = e.SrcName
			}
		case isDamage(e.Sub) && isSrc(e) && !isDst(e):
			h := Hit{Who: e.DstName, Spell: spellOf(e), Amount: e.Amount, Crit: e.Crit}
			if done == nil || h.Amount > done.Amount {
				c := h
				done = &c
			}
			if e.DstGUID != "" {
				hitTargets[e.DstGUID] = true
			}
		case isHeal(e.Sub) && isDst(e) && !isSrc(e):
			healers[e.SrcName] += e.Amount
		case e.Sub == "UNIT_DIED":
			if isDst(e) {
				if lastHit != nil {
					c := *lastHit
					ep.KilledBy = &c
				}
			} else if hitTargets[e.DstGUID] {
				kills[e.DstName]++
			}
		}
	}

	byName := map[string]int{}
	for _, n := range enemyGUIDs {
		byName[n]++
	}
	ep.EnemyCount = len(enemyGUIDs)
	ep.Enemies = sortedCounts(byName)
	ep.Kills = sortedCounts(kills)
	ep.BiggestHitTaken, ep.BiggestHitDone = taken, done
	ep.Healers = sortedAmounts(healers)
	if len(ep.Healers) > maxHealers {
		ep.Healers = ep.Healers[:maxHealers]
	}
	ep.Environmental = sortedAmounts(env)
	return ep
}

func sortedCounts(m map[string]int) []NameCount {
	out := make([]NameCount, 0, len(m))
	for n, c := range m {
		out = append(out, NameCount{n, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func sortedAmounts(m map[string]int64) []NameAmount {
	out := make([]NameAmount, 0, len(m))
	for n, a := range m {
		out = append(out, NameAmount{n, a})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Amount != out[j].Amount {
			return out[i].Amount > out[j].Amount
		}
		return out[i].Name < out[j].Name
	})
	return out
}
