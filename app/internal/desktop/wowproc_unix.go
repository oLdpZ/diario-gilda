//go:build !windows

package desktop

import (
	"fmt"
	"os/exec"
	"path"
	"strings"
)

// processNames elenca i nomi dei comandi in esecuzione con ps.
func processNames() ([]string, error) {
	out, err := exec.Command("ps", "-A", "-o", "comm=").Output()
	if err != nil {
		return nil, fmt.Errorf("elenco processi: %w", err)
	}
	var names []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			names = append(names, path.Base(l))
		}
	}
	return names, nil
}
