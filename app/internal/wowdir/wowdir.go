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
		for _, drive := range []string{"C", "D", "E"} {
			out = append(out,
				drive+`:\Program Files (x86)\World of Warcraft`,
				drive+`:\Program Files\World of Warcraft`)
		}
		return out
	case "darwin":
		return []string{"/Applications/World of Warcraft"}
	}
	return []string{}
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

// Clients restituisce le sottocartelle di root con Interface e WTF:
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
		if isDir(p) && isDir(filepath.Join(p, "Interface")) && isDir(filepath.Join(p, "WTF")) {
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
