// Package addon incorpora nell'app la cartella DiarioGilda: e' l'addon che il
// programmino installa in Interface/AddOns. Il file sta accanto alla cartella,
// non dentro, cosi' lo zip dell'addon e i test Lua non lo vedono.
package addon

import "embed"

// FS contiene la cartella DiarioGilda/ (radice: "DiarioGilda/...").
//
//go:embed all:DiarioGilda
var FS embed.FS
