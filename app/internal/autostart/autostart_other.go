//go:build !windows && !darwin

package autostart

import "errors"

// Enable non e' supportato fuori da Windows e macOS (serve solo a compilare e
// testare il resto, per esempio nella CI su Linux).
func Enable(exePath string) error {
	return errors.New("avvio automatico non supportato su questo sistema")
}

// Enabled e' sempre falso fuori da Windows e macOS.
func Enabled() bool { return false }
