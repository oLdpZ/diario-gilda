package wowdir

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

const uninstallKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

// RegistryRoots legge dal registro di Windows dove Battle.net ha installato
// WoW (anche su dischi o cartelle insolite): InstallPath di Blizzard e
// InstallLocation dell'elenco dei programmi installati.
func RegistryRoots() []string {
	var paths []string
	for _, view := range []uint32{registry.WOW64_32KEY, registry.WOW64_64KEY} {
		if v := readString(`SOFTWARE\Blizzard Entertainment\World of Warcraft`, "InstallPath", view); v != "" {
			paths = append(paths, v)
		}
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstallKey, registry.ENUMERATE_SUB_KEYS|view)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			sub := uninstallKey + `\` + n
			if !strings.HasPrefix(strings.ToLower(readString(sub, "DisplayName", view)), "world of warcraft") {
				continue
			}
			if v := readString(sub, "InstallLocation", view); v != "" {
				paths = append(paths, v)
			}
		}
	}
	return InstallRoots(paths)
}

func readString(key, name string, view uint32) string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, key, registry.QUERY_VALUE|view)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	return v
}
