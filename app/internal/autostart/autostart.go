// Package autostart fa partire l'app all'accesso dell'utente.
// Windows: valore DiarioGilda in HKCU\Software\Microsoft\Windows\CurrentVersion\Run.
// macOS: ~/Library/LaunchAgents/it.oldpz.diariogilda.plist con RunAtLoad.
package autostart

import (
	"bytes"
	"encoding/xml"
)

// Label e' l'etichetta del LaunchAgent macOS (uguale al CFBundleIdentifier).
const Label = "it.oldpz.diariogilda"

// valueName e' il nome del valore nella chiave Run di Windows.
const valueName = "DiarioGilda"

// RunValue e' il dato scritto nella chiave Run: il percorso fra virgolette.
func RunValue(exePath string) string { return `"` + exePath + `"` }

// Plist restituisce il LaunchAgent che avvia exePath all'accesso.
func Plist(exePath string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>`)
	xml.EscapeText(&b, []byte(Label))
	b.WriteString(`</string>
	<key>ProgramArguments</key>
	<array>
		<string>`)
	xml.EscapeText(&b, []byte(exePath))
	b.WriteString(`</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`)
	return b.Bytes()
}
