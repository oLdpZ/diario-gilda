package desktop

import (
	"log"
	"strings"
	"sync"

	"github.com/oLdpZ/diario-gilda/app/internal/wowdir"
)

// IsWoWProcess riconosce il nome dell'eseguibile di un client di WoW:
// Windows "Wow*.exe" (Wow, WowClassic, WowT, Wow-64...), escluse le utilita'
// di contorno; macOS "World of Warcraft*".
func IsWoWProcess(name string) bool {
	if strings.HasPrefix(strings.ToLower(name), "world of warcraft") {
		return true
	}
	return wowdir.IsGameExe(name)
}

var procErrOnce sync.Once

// WoWRunning dice se un client di WoW e' aperto. Se l'elenco dei processi non
// si legge risponde true: meglio aspettare che inviare una sessione a meta'
// dopo un /reload (la sessione parte comunque appena ne inizia una nuova).
func WoWRunning() bool { return wowRunning(processNames) }

func wowRunning(list func() ([]string, error)) bool {
	names, err := list()
	if err != nil {
		procErrOnce.Do(func() { log.Printf("elenco dei processi non leggibile, considero WoW aperto: %v", err) })
		return true
	}
	for _, n := range names {
		if IsWoWProcess(n) {
			return true
		}
	}
	return false
}
