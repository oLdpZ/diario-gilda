package desktop

import "strings"

// IsWoWProcess riconosce il nome dell'eseguibile di un client di WoW:
// Windows "Wow*.exe" (Wow, WowClassic, WowT, Wow-64...), escluse le utilita'
// di contorno; macOS "World of Warcraft*".
func IsWoWProcess(name string) bool {
	l := strings.ToLower(name)
	if strings.HasPrefix(l, "world of warcraft") {
		return true
	}
	if !strings.HasPrefix(l, "wow") || !strings.HasSuffix(l, ".exe") {
		return false
	}
	for _, helper := range []string{"wowvoiceproxy", "wowerror"} {
		if strings.HasPrefix(l, helper) {
			return false
		}
	}
	return true
}

// WoWRunning dice se un client di WoW e' aperto (false se non si sa).
func WoWRunning() bool {
	names, err := processNames()
	if err != nil {
		return false
	}
	for _, n := range names {
		if IsWoWProcess(n) {
			return true
		}
	}
	return false
}
