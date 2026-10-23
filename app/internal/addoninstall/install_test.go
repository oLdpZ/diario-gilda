package addoninstall

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

func addonFS(version string) fstest.MapFS {
	return fstest.MapFS{
		"DiarioGilda/DiarioGilda.toc": {Data: []byte("## Title: Diario di Gilda\r\n## Version: " + version + "\r\n## Interface: 16001\r\n")},
		"DiarioGilda/Main.lua":        {Data: []byte("-- main " + version + "\n")},
		"DiarioGilda/Core/Const.lua":  {Data: []byte("-- const\n")},
	}
}

func TestVersion(t *testing.T) {
	cases := []struct {
		name, toc, want string
	}{
		{"lf", "## Title: X\n## Version: 1.2.3\n", "1.2.3"},
		{"crlf", "## Title: X\r\n## Version: 0.1.0\r\n", "0.1.0"},
		{"bom", bom + "## Version: 2.0.0\n", "2.0.0"},
		{"spaces", "##   Version:   0.4.1  \n", "0.4.1"},
		{"missing", "## Title: X\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fsys := fstest.MapFS{"DiarioGilda/DiarioGilda.toc": {Data: []byte(c.toc)}}
			if got := Version(fsys); got != c.want {
				t.Fatalf("Version = %q, want %q", got, c.want)
			}
		})
	}
	if got := Version(fstest.MapFS{}); got != "" {
		t.Fatalf("Version(empty) = %q", got)
	}
}

func TestEmbeddedHasVersion(t *testing.T) {
	v := Version(Embedded)
	if v == "" {
		t.Fatal("embedded addon has no ## Version")
	}
	if _, err := fs.Stat(Embedded, "DiarioGilda/Main.lua"); err != nil {
		t.Fatalf("embedded Main.lua: %v", err)
	}
}

func TestInstallEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if !NeedsInstall(Embedded, dir) {
		t.Fatal("NeedsInstall on empty dir = false")
	}
	if err := Install(Embedded, dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"DiarioGilda.toc", "Main.lua", filepath.Join("Core", "Const.lua")} {
		if _, err := os.Stat(filepath.Join(dir, "DiarioGilda", f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
	if got, want := Installed(dir), Version(Embedded); got != want {
		t.Fatalf("Installed = %q, want %q", got, want)
	}
	if NeedsInstall(Embedded, dir) {
		t.Fatal("NeedsInstall after install = true")
	}
	assertNoLeftovers(t, dir)
}

func TestInstallReplacesOtherVersion(t *testing.T) {
	dir := t.TempDir()
	if err := Install(addonFS("0.0.9"), dir); err != nil {
		t.Fatal(err)
	}
	// un file che la nuova versione non ha deve sparire
	stale := filepath.Join(dir, "DiarioGilda", "Vecchio.lua")
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	next := addonFS("0.2.0")
	if !NeedsInstall(next, dir) {
		t.Fatal("NeedsInstall with different version = false")
	}
	if err := Install(next, dir); err != nil {
		t.Fatal(err)
	}
	if got := Installed(dir); got != "0.2.0" {
		t.Fatalf("Installed = %q", got)
	}
	if _, err := os.Stat(stale); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stale file still present: %v", err)
	}
	assertNoLeftovers(t, dir)
}

func TestInstallReadOnlyKeepsPrevious(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("su Windows l'attributo sola lettura non protegge le cartelle; vedi i test con file aperto e rename finto")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignora i permessi")
	}
	dir := t.TempDir()
	if err := Install(addonFS("0.1.0"), dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if err := Install(addonFS("0.2.0"), dir); err == nil {
		t.Fatal("Install in read-only dir: no error")
	}
	if got := Installed(dir); got != "0.1.0" {
		t.Fatalf("previous version not intact: %q", got)
	}
}

func TestInstallLockedFileKeepsPrevious(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("solo Windows blocca il rename di una cartella con un file aperto")
	}
	dir := t.TempDir()
	if err := Install(addonFS("0.1.0"), dir); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(dir, "DiarioGilda", "Main.lua"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Install(addonFS("0.2.0"), dir); err == nil {
		t.Fatal("Install with locked folder: no error")
	}
	if got := Installed(dir); got != "0.1.0" {
		t.Fatalf("previous version not intact: %q", got)
	}
	f.Close()
	assertNoLeftovers(t, dir)
}

// errFS fallisce leggendo un file: la copia si interrompe a meta'.
type errFS struct{ fstest.MapFS }

func (e errFS) Open(name string) (fs.File, error) {
	if name == "DiarioGilda/Main.lua" {
		return nil, errors.New("disco rotto")
	}
	return e.MapFS.Open(name)
}

func TestInstallCopyFailureKeepsPrevious(t *testing.T) {
	dir := t.TempDir()
	if err := Install(addonFS("0.1.0"), dir); err != nil {
		t.Fatal(err)
	}
	if err := Install(errFS{addonFS("0.2.0")}, dir); err == nil {
		t.Fatal("no error")
	}
	if got := Installed(dir); got != "0.1.0" {
		t.Fatalf("previous version not intact: %q", got)
	}
	assertNoLeftovers(t, dir)
}

func TestInstallSwapFailureRestoresOld(t *testing.T) {
	dir := t.TempDir()
	if err := Install(addonFS("0.1.0"), dir); err != nil {
		t.Fatal(err)
	}
	calls := 0
	rename := func(from, to string) error {
		calls++
		if strings.HasSuffix(from, ".new") {
			return errors.New("rename finto fallito")
		}
		return os.Rename(from, to)
	}
	if err := install(addonFS("0.2.0"), dir, rename); err == nil {
		t.Fatal("no error")
	}
	if calls < 2 {
		t.Fatalf("rename called %d times", calls)
	}
	if got := Installed(dir); got != "0.1.0" {
		t.Fatalf("previous version not restored: %q", got)
	}
	assertNoLeftovers(t, dir)
}

func TestInstallRecoversInterruptedSwap(t *testing.T) {
	// crash fra i due rename: c'e' solo DiarioGilda.old
	dir := t.TempDir()
	if err := Install(addonFS("0.1.0"), dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "DiarioGilda"), filepath.Join(dir, "DiarioGilda.old")); err != nil {
		t.Fatal(err)
	}
	if err := Install(errFS{addonFS("0.2.0")}, dir); err == nil {
		t.Fatal("no error")
	}
	if got := Installed(dir); got != "0.1.0" {
		t.Fatalf("old copy not recovered: %q", got)
	}
}

func TestInstallRejectsFSWithoutVersion(t *testing.T) {
	dir := t.TempDir()
	if err := Install(addonFS("0.1.0"), dir); err != nil {
		t.Fatal(err)
	}
	bad := fstest.MapFS{"DiarioGilda/Main.lua": {Data: []byte("x")}}
	if err := Install(bad, dir); err == nil {
		t.Fatal("no error")
	}
	if NeedsInstall(bad, dir) {
		t.Fatal("NeedsInstall with versionless fs = true")
	}
	if got := Installed(dir); got != "0.1.0" {
		t.Fatalf("Installed = %q", got)
	}
}

func TestInstalledMissing(t *testing.T) {
	if got := Installed(t.TempDir()); got != "" {
		t.Fatalf("Installed = %q", got)
	}
}

func assertNoLeftovers(t *testing.T, dir string) {
	t.Helper()
	for _, s := range []string{".new", ".old"} {
		if _, err := os.Stat(filepath.Join(dir, "DiarioGilda"+s)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("DiarioGilda%s left behind (err=%v)", s, err)
		}
	}
}

const bom = "\ufeff"
