package wowdir

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func mk(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaultRoots(t *testing.T) {
	win := DefaultRoots("windows")
	want := []string{
		`C:\Program Files (x86)\World of Warcraft`, `C:\Program Files\World of Warcraft`,
		`D:\Program Files (x86)\World of Warcraft`, `D:\Program Files\World of Warcraft`,
		`E:\Program Files (x86)\World of Warcraft`, `E:\Program Files\World of Warcraft`,
	}
	if !reflect.DeepEqual(win, want) {
		t.Fatalf("windows: %v", win)
	}
	if got := DefaultRoots("darwin"); !reflect.DeepEqual(got, []string{"/Applications/World of Warcraft"}) {
		t.Fatalf("darwin: %v", got)
	}
	if got := DefaultRoots("linux"); len(got) != 0 {
		t.Fatalf("linux: %v", got)
	}
}

func wf(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func client(t *testing.T, root, name string) {
	mk(t, root, name, "Interface")
	mk(t, root, name, "WTF")
}

func TestClientsOrder(t *testing.T) {
	root := t.TempDir()
	client(t, root, "_retail_")
	client(t, root, "_classic_beta_")
	client(t, root, "_Forever_")
	client(t, root, "_anniversary_")
	mk(t, root, "_ptr_", "Interface") // solo Interface: esclusa
	wf(t, filepath.Join(root, "file"))
	got := Clients(root)
	want := []string{
		filepath.Join(root, "_Forever_"), filepath.Join(root, "_classic_beta_"),
		filepath.Join(root, "_anniversary_"), filepath.Join(root, "_retail_"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestClientsEmptyAndMissing(t *testing.T) {
	if got := Clients(t.TempDir()); got == nil || len(got) != 0 {
		t.Fatalf("empty: %#v", got)
	}
	if got := Clients(filepath.Join(t.TempDir(), "nope")); len(got) != 0 {
		t.Fatalf("missing: %v", got)
	}
}

func TestSavedVariables(t *testing.T) {
	c := t.TempDir()
	var want []string
	for _, acc := range []string{"B", "A"} {
		d := mk(t, c, "WTF", "Account", acc, "SavedVariables")
		f := filepath.Join(d, "DiarioGilda.lua")
		wf(t, f)
		wf(t, filepath.Join(d, "DiarioGilda.lua.bak"))
	}
	want = []string{
		filepath.Join(c, "WTF", "Account", "A", "SavedVariables", "DiarioGilda.lua"),
		filepath.Join(c, "WTF", "Account", "B", "SavedVariables", "DiarioGilda.lua"),
	}
	// account senza file, e directory con il nome del file
	mk(t, c, "WTF", "Account", "C", "SavedVariables")
	mk(t, c, "WTF", "Account", "D", "SavedVariables", "DiarioGilda.lua")
	got := SavedVariables(c)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := SavedVariables(t.TempDir()); got == nil || len(got) != 0 {
		t.Fatalf("empty: %#v", got)
	}
}

func TestDirs(t *testing.T) {
	c := filepath.Join("x", "_forever_")
	if AddOnsDir(c) != filepath.Join(c, "Interface", "AddOns") {
		t.Fatal(AddOnsDir(c))
	}
	if LogsDir(c) != filepath.Join(c, "Logs") {
		t.Fatal(LogsDir(c))
	}
}

func TestSavedVariablesSpecialChars(t *testing.T) {
	names := []string{"w[ow]"}
	if runtime.GOOS != "windows" {
		names = append(names, "w*ow", "w?ow")
	}
	for _, n := range names {
		c := filepath.Join(t.TempDir(), n)
		d := mk(t, c, "WTF", "Account", "A", "SavedVariables")
		f := filepath.Join(d, "DiarioGilda.lua")
		wf(t, f)
		if got := SavedVariables(c); !reflect.DeepEqual(got, []string{f}) {
			t.Fatalf("%s: got %v", n, got)
		}
	}
}

func TestClientsTieCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	client(t, root, "_b_")
	client(t, root, "_A_")
	client(t, root, "_c_")
	got := Clients(root)
	want := []string{filepath.Join(root, "_A_"), filepath.Join(root, "_b_"), filepath.Join(root, "_c_")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestIsForever(t *testing.T) {
	for _, tc := range []struct {
		dir  string
		want bool
	}{
		{filepath.Join("C:", "WoW", "_forever_"), true},
		{filepath.Join("WoW", "_Forever_"), true},
		{filepath.Join("WoW", "WoW Forever"), true},
		{filepath.Join("WoW", "_classic_beta_"), true},
		{filepath.Join("WoW", "_classic_"), false},
		{filepath.Join("WoW", "_retail_"), false},
		{filepath.Join("forever", "_classic_"), false}, // conta solo il nome del client
		{"", false},
	} {
		if got := IsForever(tc.dir); got != tc.want {
			t.Errorf("IsForever(%q) = %v", tc.dir, got)
		}
	}
}
