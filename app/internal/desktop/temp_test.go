package desktop

import (
	"path/filepath"
	"testing"
)

func TestInTempDir(t *testing.T) {
	tmp := filepath.FromSlash("/home/gino/AppData/Local/Temp")
	si := []string{
		// Esplora risorse: doppio clic dentro lo zip
		"/home/gino/AppData/Local/Temp/Temp1_DiarioGilda-windows-amd64.zip/DiarioGilda.exe",
		// 7-Zip / WinRAR aprono in una sottocartella della Temp
		"/home/gino/AppData/Local/Temp/7zO8A1B2C3D/DiarioGilda.exe",
		"/home/gino/APPDATA/local/TEMP/Rar$EXa1234.5678/DiarioGilda.exe", // Windows non distingue maiuscole
	}
	no := []string{
		"/home/gino/Documents/DiarioGilda/DiarioGilda.exe",
		"/home/gino/Downloads/DiarioGilda-windows-amd64/DiarioGilda.exe",
		"/home/gino/AppData/Local/TempOther/DiarioGilda.exe", // stesso inizio, cartella diversa
		"/home/gino/AppData/Local/Temp",                      // la cartella stessa non è un programma dentro
	}
	for _, p := range si {
		if !InTempDir(filepath.FromSlash(p), tmp) {
			t.Errorf("%s: dovrebbe essere nella cartella temporanea", p)
		}
	}
	for _, p := range no {
		if InTempDir(filepath.FromSlash(p), tmp) {
			t.Errorf("%s: non è nella cartella temporanea", p)
		}
	}
	if InTempDir(filepath.FromSlash("/x/DiarioGilda.exe"), "") {
		t.Error("senza cartella temporanea nota: mai temporanea")
	}
}
