package desktop

import (
	"path/filepath"
	"strings"
)

// MsgExtractZip: il programma gira da dentro lo zip (Windows lo ha copiato nella cartella temporanea).
const MsgExtractZip = "Hai aperto il Diario di Gilda da dentro lo zip: Windows lo fa girare da una " +
	"cartella temporanea che poi cancella, quindi non potrà partire da solo all'accensione del PC.\n\n" +
	"Chiudilo dall'icona vicino all'orologio, poi fai tasto destro sullo zip → «Estrai tutto…», " +
	"metti la cartella in un posto fisso (per esempio Documenti) e apri il Diario di Gilda da lì."

// InTempDir dice se exe sta dentro la cartella temporanea tmp (anche in una sua sottocartella):
// succede su Windows quando si apre il programma con un doppio clic dentro lo zip, senza estrarlo.
// Il confronto ignora maiuscole e minuscole, come Windows.
func InTempDir(exe, tmp string) bool {
	if tmp == "" || exe == "" {
		return false
	}
	t := strings.ToLower(filepath.Clean(tmp))
	e := strings.ToLower(filepath.Clean(exe))
	sep := string(filepath.Separator)
	return strings.HasPrefix(e, strings.TrimSuffix(t, sep)+sep)
}
