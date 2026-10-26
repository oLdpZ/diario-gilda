// Package wowdir trova l'installazione di WoW, il client Forever e i file dell'addon.
package wowdir

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultRoots elenca le cartelle di installazione abituali per il sistema indicato.
func DefaultRoots(goos string) []string {
	switch goos {
	case "windows":
		var out []string
		for _, drive := range []string{"C", "D", "E", "F"} {
			out = append(out,
				drive+`:\Program Files (x86)\World of Warcraft`,
				drive+`:\Program Files\World of Warcraft`,
				drive+`:\World of Warcraft`,
				drive+`:\WoW\World of Warcraft`,
				drive+`:\Games\World of Warcraft`,
				drive+`:\Giochi\World of Warcraft`)
		}
		return out
	case "darwin":
		return []string{"/Applications/World of Warcraft"}
	}
	return []string{}
}

// Roots sono le cartelle dove cercare WoW: prima quelle lette dal registro
// (extra), poi quelle abituali, senza doppioni.
func Roots(goos string, extra []string) []string {
	return InstallRoots(append(append([]string{}, extra...), DefaultRoots(goos)...))
}

// InstallRoots porta i percorsi d'installazione alla cartella che contiene i
// client (un percorso che finisce in un client come _retail_ diventa la
// cartella sopra), togliendo vuoti e doppioni (maiuscole comprese).
func InstallRoots(paths []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		if b := filepath.Base(p); len(b) > 2 && strings.HasPrefix(b, "_") && strings.HasSuffix(b, "_") {
			p = filepath.Dir(p)
		}
		if k := strings.ToLower(p); !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	return out
}

// IsGameExe riconosce l'eseguibile di un client di WoW per Windows
// ("Wow*.exe": Wow, WowClassic, WowT, Wow-64...), escluse le utilita' di contorno.
func IsGameExe(name string) bool {
	l := strings.ToLower(name)
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

// IsClient dice se dir e' la cartella di un client: ha Interface e WTF, oppure
// contiene il gioco (Wow*.exe, o "World of Warcraft*.app" su macOS). Un client
// scaricato ma mai aperto non ha ancora WTF, e spesso nemmeno Interface.
func IsClient(dir string) bool {
	if isDir(filepath.Join(dir, "Interface")) && isDir(filepath.Join(dir, "WTF")) {
		return true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		l := strings.ToLower(e.Name())
		if strings.Contains(l, "launcher") {
			continue
		}
		if e.Type().IsRegular() && IsGameExe(e.Name()) {
			return true
		}
		if e.IsDir() && strings.HasPrefix(l, "world of warcraft") && strings.HasSuffix(l, ".app") {
			return true
		}
	}
	return false
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func rank(name string) int {
	l := strings.ToLower(name)
	switch {
	case strings.Contains(l, "forever"):
		return 0
	case l == "_classic_beta_":
		return 1
	}
	return 2
}

// IsForever dice se il nome della cartella del client e' quello di WoW Forever
// (contiene "forever", o e' _classic_beta_): solo questi si scelgono da soli.
func IsForever(clientDir string) bool {
	if clientDir == "" {
		return false
	}
	return rank(filepath.Base(clientDir)) < 2
}

// Clients restituisce le sottocartelle di root che sono client (vedi IsClient):
// prima "forever", poi _classic_beta_, poi le altre (a pari rango, ordine alfabetico).
func Clients(root string) []string {
	out := []string{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	var names []string
	for _, e := range entries {
		p := filepath.Join(root, e.Name())
		if isDir(p) && IsClient(p) {
			names = append(names, e.Name())
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		ri, rj := rank(names[i]), rank(names[j])
		if ri != rj {
			return ri < rj
		}
		li, lj := strings.ToLower(names[i]), strings.ToLower(names[j])
		if li != lj {
			return li < lj
		}
		return names[i] < names[j]
	})
	for _, n := range names {
		out = append(out, filepath.Join(root, n))
	}
	return out
}

// SavedVariables restituisce i file DiarioGilda.lua esistenti (solo file regolari) in tutti gli account.
// Non usa glob: il percorso scelto dall'utente può contenere caratteri speciali come [ o *.
func SavedVariables(clientDir string) []string {
	out := []string{}
	accounts, err := os.ReadDir(filepath.Join(clientDir, "WTF", "Account"))
	if err != nil {
		return out
	}
	for _, a := range accounts { // ReadDir è già ordinato per nome
		f := filepath.Join(clientDir, "WTF", "Account", a.Name(), "SavedVariables", "DiarioGilda.lua")
		if st, err := os.Stat(f); err == nil && st.Mode().IsRegular() {
			out = append(out, f)
		}
	}
	return out
}

// AddOnsDir è la cartella Interface/AddOns del client.
func AddOnsDir(clientDir string) string {
	return filepath.Join(clientDir, "Interface", "AddOns")
}

// LogsDir è la cartella Logs del client.
func LogsDir(clientDir string) string {
	return filepath.Join(clientDir, "Logs")
}
