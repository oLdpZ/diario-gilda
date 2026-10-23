package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// releaseServer serve /repos/{repo}/releases/latest con il tag dato e gli asset
// di entrambe le piattaforme, piu' i file sotto /dl/.
func releaseServer(t *testing.T, tag string, files map[string][]byte, extra ...map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var ts *httptest.Server
	mux.HandleFunc("/repos/oLdpZ/diario-gilda/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		var as []asset
		for _, n := range []string{assetWindows, assetMac} {
			as = append(as, asset{n, ts.URL + "/dl/" + n}, asset{n + ".sig", ts.URL + "/dl/" + n + ".sig"})
		}
		body := map[string]any{"tag_name": tag, "assets": as}
		for _, e := range extra {
			for k, v := range e {
				body[k] = v
			}
		}
		json.NewEncoder(w).Encode(body)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[strings.TrimPrefix(r.URL.Path, "/dl/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	ts = httptest.NewTLSServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestCheck(t *testing.T) {
	cases := []struct {
		tag, current, goos string
		wantVer, wantAsset string // wantVer "" = nil
	}{
		{"v0.2.0", "0.1.0", "windows", "0.2.0", assetWindows},
		{"v0.2.0", "0.1.0", "darwin", "0.2.0", assetMac},
		{"0.2.0", "v0.1.9", "windows", "0.2.0", assetWindows},
		{"v0.10.0", "0.9.0", "windows", "0.10.0", assetWindows},
		{"v1.0.0", "0.99.99", "windows", "1.0.0", assetWindows},
		{"v0.1.0", "0.1.0", "windows", "", ""},
		{"v0.0.9", "0.1.0", "windows", "", ""},
		{"v0.2.0", "dev", "windows", "", ""},
		{"v0.2.0", "", "windows", "", ""},
		// pre-release: saltate se la versione attuale non e' pre-release
		{"v0.1.1-test", "0.1.0", "windows", "", ""},
		{"v0.1.1-test", "0.1.0-test", "windows", "0.1.1-test", assetWindows},
		{"v0.1.0", "0.1.0-rc1", "windows", "0.1.0", assetWindows},
		{"v0.1.0-rc2", "0.1.0-rc1", "windows", "", ""},
	}
	for _, c := range cases {
		t.Run(c.tag+"_"+c.current+"_"+c.goos, func(t *testing.T) {
			ts := releaseServer(t, c.tag, nil)
			rel, err := check(context.Background(), ts.Client(), ts.URL, "oLdpZ/diario-gilda", c.current, c.goos, testKey)
			if err != nil {
				t.Fatal(err)
			}
			if c.wantVer == "" {
				if rel != nil {
					t.Fatalf("want nil, got %+v", rel)
				}
				return
			}
			if rel == nil {
				t.Fatal("got nil")
			}
			if rel.Version != c.wantVer {
				t.Fatalf("Version = %q", rel.Version)
			}
			if rel.AssetURL != ts.URL+"/dl/"+c.wantAsset || rel.SigURL != ts.URL+"/dl/"+c.wantAsset+".sig" {
				t.Fatalf("urls = %q %q", rel.AssetURL, rel.SigURL)
			}
		})
	}
}

const testKey = "chiave-di-test" // check controlla solo che non sia vuota

func TestCheckRuntimeGOOS(t *testing.T) {
	ts := releaseServer(t, "v9.0.0", nil)
	// build di sviluppo: build.PublicKey vuota, quindi mai aggiornamenti
	rel, err := Check(context.Background(), ts.Client(), ts.URL, "oLdpZ/diario-gilda", "0.1.0")
	if err != nil || rel != nil {
		t.Fatalf("Check without key: rel=%v err=%v", rel, err)
	}
	want, err := assetName(goosForTest())
	if err != nil {
		t.Skip("GOOS non supportato")
	}
	rel, err = check(context.Background(), ts.Client(), ts.URL, "oLdpZ/diario-gilda", "0.1.0", goosForTest(), testKey)
	if err != nil || rel == nil {
		t.Fatalf("rel=%v err=%v", rel, err)
	}
	if !strings.HasSuffix(rel.AssetURL, "/"+want) {
		t.Fatalf("AssetURL = %q", rel.AssetURL)
	}
}

func TestCheckSkips(t *testing.T) {
	cases := map[string]map[string]any{
		"prerelease": {"prerelease": true},
		"draft":      {"draft": true},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			ts := releaseServer(t, "v0.2.0", nil, extra)
			rel, err := check(context.Background(), ts.Client(), ts.URL, "oLdpZ/diario-gilda", "0.1.0", "windows", testKey)
			if err != nil || rel != nil {
				t.Fatalf("rel=%v err=%v", rel, err)
			}
		})
	}
	t.Run("no key", func(t *testing.T) {
		ts := releaseServer(t, "v0.2.0", nil)
		rel, err := check(context.Background(), ts.Client(), ts.URL, "oLdpZ/diario-gilda", "0.1.0", "windows", "")
		if err != nil || rel != nil {
			t.Fatalf("rel=%v err=%v", rel, err)
		}
	})
}

func TestCheckErrors(t *testing.T) {
	t.Run("missing sig", func(t *testing.T) {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"tag_name":"v0.2.0","assets":[{"name":%q,"browser_download_url":"https://x/a"}]}`, assetWindows)
		}))
		defer ts.Close()
		if _, err := check(context.Background(), ts.Client(), ts.URL, "o/r", "0.1.0", "windows", testKey); err == nil {
			t.Fatal("no error")
		}
	})
	t.Run("http asset url", func(t *testing.T) {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"tag_name":"v0.2.0","assets":[{"name":%q,"browser_download_url":"http://x/a"},{"name":%q,"browser_download_url":"https://x/a.sig"}]}`, assetWindows, assetWindows+".sig")
		}))
		defer ts.Close()
		if _, err := check(context.Background(), ts.Client(), ts.URL, "o/r", "0.1.0", "windows", testKey); err == nil {
			t.Fatal("no error")
		}
	})
	t.Run("http api base", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer ts.Close()
		if _, err := check(context.Background(), ts.Client(), ts.URL, "o/r", "0.1.0", "windows", testKey); err == nil {
			t.Fatal("no error")
		}
	})
	t.Run("status 500", func(t *testing.T) {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", 500)
		}))
		defer ts.Close()
		if _, err := check(context.Background(), ts.Client(), ts.URL, "o/r", "0.1.0", "windows", testKey); err == nil {
			t.Fatal("no error")
		}
	})
	t.Run("no releases", func(t *testing.T) {
		ts := httptest.NewTLSServer(http.NotFoundHandler())
		defer ts.Close()
		rel, err := check(context.Background(), ts.Client(), ts.URL, "o/r", "0.1.0", "windows", testKey)
		if err != nil || rel != nil {
			t.Fatalf("rel=%v err=%v", rel, err)
		}
	})
	t.Run("bad tag", func(t *testing.T) {
		ts := releaseServer(t, "latest", nil)
		if _, err := check(context.Background(), ts.Client(), ts.URL, "oLdpZ/diario-gilda", "0.1.0", "windows", testKey); err == nil {
			t.Fatal("no error")
		}
	})
	t.Run("unsupported goos", func(t *testing.T) {
		ts := releaseServer(t, "v0.2.0", nil)
		if _, err := check(context.Background(), ts.Client(), ts.URL, "oLdpZ/diario-gilda", "0.1.0", "plan9", testKey); err == nil {
			t.Fatal("no error")
		}
	})
}

func keys(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pub), priv
}

func sign(priv ed25519.PrivateKey, data []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n")
}

func TestVerify(t *testing.T) {
	pub, priv := keys(t)
	otherPub, _ := keys(t)
	data := []byte("contenuto dello zip")
	sig := sign(priv, data)
	if err := Verify(data, sig, pub); err != nil {
		t.Fatalf("good signature: %v", err)
	}
	tampered := append([]byte(nil), data...)
	tampered[3] ^= 1
	cases := map[string]struct {
		data, sig []byte
		pub       string
	}{
		"byte changed": {tampered, sig, pub},
		"truncated":    {data[:len(data)-1], sig, pub},
		"other key":    {data, sig, otherPub},
		"empty key":    {data, sig, ""},
		"bad key b64":  {data, sig, "!!!"},
		"short key":    {data, sig, base64.StdEncoding.EncodeToString([]byte("corta"))},
		"bad sig b64":  {data, []byte("%%%"), pub},
		"short sig":    {data, []byte(base64.StdEncoding.EncodeToString([]byte("x"))), pub},
		"empty sig":    {data, nil, pub},
	}
	for name, c := range cases {
		if err := Verify(c.data, c.sig, c.pub); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fakeExe(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyWindows(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "DiarioGilda.exe")
	fakeExe(t, exe, "vecchio")
	z := makeZip(t, map[string]string{"DiarioGilda.exe": "nuovo", "LEGGIMI.txt": "ignorato"})
	if err := apply(z, "windows", exe); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, exe); got != "nuovo" {
		t.Fatalf("exe = %q", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "LEGGIMI.txt")); err == nil {
		t.Fatal("unexpected entry extracted")
	}
}

func macBundle(t *testing.T) (root, bundle, exe string) {
	t.Helper()
	root = t.TempDir()
	bundle = filepath.Join(root, "DiarioGilda.app")
	exe = filepath.Join(bundle, "Contents", "MacOS", "DiarioGilda")
	fakeExe(t, exe, "vecchio")
	fakeExe(t, filepath.Join(bundle, "Contents", "Info.plist"), "plist vecchio")
	fakeExe(t, filepath.Join(bundle, "Contents", "Resources", "vecchia.icns"), "vecchia")
	return root, bundle, exe
}

func macZip(t *testing.T) []byte {
	return makeZip(t, map[string]string{
		"VERSION": "0.2.0\n",
		"DiarioGilda.app/Contents/MacOS/DiarioGilda":    "nuovo",
		"DiarioGilda.app/Contents/Info.plist":           "plist nuovo",
		"DiarioGilda.app/Contents/Resources/icona.icns": "icona",
		"altro/fuori.txt": "ignorato",
	})
}

func TestApplyMacSwapsBundle(t *testing.T) {
	root, bundle, exe := macBundle(t)
	if err := apply(macZip(t), "darwin", exe); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, exe); got != "nuovo" {
		t.Fatalf("exe = %q", got)
	}
	if got := readFile(t, filepath.Join(bundle, "Contents", "Info.plist")); got != "plist nuovo" {
		t.Fatalf("plist = %q", got)
	}
	if got := readFile(t, filepath.Join(bundle, "Contents", "Resources", "icona.icns")); got != "icona" {
		t.Fatalf("icon = %q", got)
	}
	// bundle sostituito per intero: niente file vecchi dentro
	if _, err := os.Stat(filepath.Join(bundle, "Contents", "Resources", "vecchia.icns")); err == nil {
		t.Fatal("old resource left in the new bundle")
	}
	for _, p := range []string{filepath.Join(root, "altro"), filepath.Join(bundle, "VERSION"), bundle + ".new"} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s should not exist", p)
		}
	}
	if got := readFile(t, filepath.Join(bundle+".old", "Contents", "MacOS", "DiarioGilda")); got != "vecchio" {
		t.Fatalf("old bundle = %q", got)
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(exe); err != nil || fi.Mode().Perm()&0o111 == 0 {
			t.Fatalf("exe not executable: %v %v", fi, err)
		}
	}
	cleanupOld("darwin", exe)
	if _, err := os.Stat(bundle + ".old"); err == nil {
		t.Fatal("CleanupOld left DiarioGilda.app.old")
	}
}

func TestApplyMacRollback(t *testing.T) {
	files, err := unzip(macZip(t))
	if err != nil {
		t.Fatal(err)
	}
	_, bundle, exe := macBundle(t)
	rename := func(from, to string) error {
		if strings.HasSuffix(from, ".new") {
			return fmt.Errorf("rename finto fallito")
		}
		return os.Rename(from, to)
	}
	if err := applyFiles(files, "darwin", exe, rename); err == nil {
		t.Fatal("no error")
	}
	if got := readFile(t, exe); got != "vecchio" {
		t.Fatalf("exe = %q", got)
	}
	if got := readFile(t, filepath.Join(bundle, "Contents", "Resources", "vecchia.icns")); got != "vecchia" {
		t.Fatalf("old bundle not intact: %q", got)
	}
	for _, p := range []string{bundle + ".new", bundle + ".old"} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s left behind", p)
		}
	}
}

func TestApplyMacNotInBundle(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "DiarioGilda")
	fakeExe(t, exe, "vecchio")
	if err := apply(macZip(t), "darwin", exe); err == nil {
		t.Fatal("no error")
	}
	if got := readFile(t, exe); got != "vecchio" {
		t.Fatalf("exe = %q", got)
	}
}

func TestCleanupOldWindows(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "DiarioGilda.exe")
	fakeExe(t, exe+".old", "vecchio")
	cleanupOld("windows", exe)
	if _, err := os.Stat(exe + ".old"); err == nil {
		t.Fatal(".old left")
	}
}

func TestApplyRejects(t *testing.T) {
	cases := map[string][]byte{
		"not a zip": []byte("PK questo non e' uno zip"),
		"no exe":    makeZip(t, map[string]string{"altro.exe": "x"}),
		"zip slip":  makeZip(t, map[string]string{"DiarioGilda.exe": "x", "../fuori.exe": "x"}),
		"absolute":  makeZip(t, map[string]string{"DiarioGilda.exe": "x", "/etc/passwd": "x"}),
		"drive":     makeZip(t, map[string]string{"DiarioGilda.exe": "x", "C:/Windows/x": "x"}),
		"backslash": makeZip(t, map[string]string{"DiarioGilda.exe": "x", `..\fuori.exe`: "x"}),
		"empty exe": makeZip(t, map[string]string{"DiarioGilda.exe": ""}),
		"duplicate": dupZip(t),
		"symlink":   symlinkZip(t),
	}
	for name, z := range cases {
		t.Run(name, func(t *testing.T) {
			exe := filepath.Join(t.TempDir(), "DiarioGilda.exe")
			fakeExe(t, exe, "vecchio")
			if err := apply(z, "windows", exe); err == nil {
				t.Fatal("no error")
			}
			if got := readFile(t, exe); got != "vecchio" {
				t.Fatalf("exe changed: %q", got)
			}
		})
	}
}

func symlinkZip(t *testing.T) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("DiarioGilda.exe")
	w.Write([]byte("nuovo"))
	h := &zip.FileHeader{Name: "link", Method: zip.Store}
	h.SetMode(os.ModeSymlink | 0o777)
	w, _ = zw.CreateHeader(h)
	w.Write([]byte("../../etc/passwd"))
	zw.Close()
	return buf.Bytes()
}

func dupZip(t *testing.T) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, b := range []string{"uno", "due"} {
		w, _ := zw.Create("DiarioGilda.exe")
		w.Write([]byte(b))
	}
	zw.Close()
	return buf.Bytes()
}

func TestApplyTooBig(t *testing.T) {
	// zip piccolo che si espande oltre il limite
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("DiarioGilda.exe")
	chunk := make([]byte, 1<<20)
	for i := 0; i < maxUnzipped>>20+1; i++ {
		w.Write(chunk)
	}
	zw.Close()
	exe := filepath.Join(t.TempDir(), "DiarioGilda.exe")
	fakeExe(t, exe, "vecchio")
	if err := apply(buf.Bytes(), "windows", exe); err == nil {
		t.Fatal("no error")
	}
	if got := readFile(t, exe); got != "vecchio" {
		t.Fatalf("exe changed: %q", got)
	}
}

func TestUpdateEndToEnd(t *testing.T) {
	pub, priv := keys(t)
	z := makeZip(t, map[string]string{"VERSION": "0.2.0\n", "DiarioGilda.exe": "nuovo"})
	signed := func(z []byte) map[string][]byte {
		return map[string][]byte{assetWindows: z, assetWindows + ".sig": sign(priv, z)}
	}

	run := func(t *testing.T, tag string, files map[string][]byte, key string) (string, error) {
		ts := releaseServer(t, tag, files)
		rel, err := check(context.Background(), ts.Client(), ts.URL, "oLdpZ/diario-gilda", "0.1.0", "windows", testKey)
		if err != nil || rel == nil {
			t.Fatalf("check: rel=%v err=%v", rel, err)
		}
		exe := filepath.Join(t.TempDir(), "DiarioGilda.exe")
		fakeExe(t, exe, "vecchio")
		err = install(context.Background(), ts.Client(), rel, "0.1.0", key, "windows", exe)
		return readFile(t, exe), err
	}
	ok := func(t *testing.T, got string, err error) {
		t.Helper()
		if err != nil || got != "nuovo" {
			t.Fatalf("got=%q err=%v", got, err)
		}
	}
	rejected := func(t *testing.T, got string, err error) {
		t.Helper()
		if err == nil || got != "vecchio" {
			t.Fatalf("want rejection, got=%q err=%v", got, err)
		}
	}

	t.Run("ok", func(t *testing.T) {
		got, err := run(t, "v0.2.0", signed(z), pub)
		ok(t, got, err)
	})
	t.Run("no public key", func(t *testing.T) {
		got, err := run(t, "v0.2.0", signed(z), "")
		rejected(t, got, err)
	})
	t.Run("tampered", func(t *testing.T) {
		bad := append([]byte(nil), z...)
		bad[len(bad)/2] ^= 1
		got, err := run(t, "v0.2.0", map[string][]byte{assetWindows: bad, assetWindows + ".sig": sign(priv, z)}, pub)
		rejected(t, got, err)
	})
	t.Run("half downloaded", func(t *testing.T) {
		got, err := run(t, "v0.2.0", map[string][]byte{assetWindows: z[:len(z)/2], assetWindows + ".sig": sign(priv, z)}, pub)
		rejected(t, got, err)
	})
	t.Run("missing sig file", func(t *testing.T) {
		got, err := run(t, "v0.2.0", map[string][]byte{assetWindows: z}, pub)
		rejected(t, got, err)
	})
	t.Run("old signed zip under new tag", func(t *testing.T) {
		old := makeZip(t, map[string]string{"VERSION": "0.1.0\n", "DiarioGilda.exe": "nuovo"})
		got, err := run(t, "v0.2.0", signed(old), pub)
		rejected(t, got, err)
	})
	t.Run("older signed zip under fake tag", func(t *testing.T) {
		got, err := run(t, "v99.0.0", signed(z), pub) // VERSION 0.2.0 != 99.0.0
		rejected(t, got, err)
	})
	t.Run("missing VERSION", func(t *testing.T) {
		nov := makeZip(t, map[string]string{"DiarioGilda.exe": "nuovo"})
		got, err := run(t, "v0.2.0", signed(nov), pub)
		rejected(t, got, err)
	})
	t.Run("VERSION not newer", func(t *testing.T) {
		// VERSION coerente col tag ma non piu' nuovo della versione attuale passata a install
		ts := releaseServer(t, "v0.2.0", signed(z))
		rel, _ := check(context.Background(), ts.Client(), ts.URL, "oLdpZ/diario-gilda", "0.1.0", "windows", testKey)
		exe := filepath.Join(t.TempDir(), "DiarioGilda.exe")
		fakeExe(t, exe, "vecchio")
		err := install(context.Background(), ts.Client(), rel, "0.2.0", pub, "windows", exe)
		rejected(t, readFile(t, exe), err)
	})
}

func TestRedirectToHTTPRejected(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("dati"))
	}))
	defer plain.Close()
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/x", http.StatusFound)
	}))
	defer ts.Close()
	if _, err := download(context.Background(), ts.Client(), ts.URL, 1024); err == nil {
		t.Fatal("redirect to http followed")
	}
	if _, err := download(context.Background(), nil, ts.URL, 1024); err == nil {
		t.Fatal("redirect to http followed (default client)")
	}
}

func TestDownloadLimits(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 2048))
	}))
	defer ts.Close()
	if _, err := download(context.Background(), ts.Client(), ts.URL, 1024); err == nil {
		t.Fatal("oversize: no error")
	}
	if b, err := download(context.Background(), ts.Client(), ts.URL, 4096); err != nil || len(b) != 2048 {
		t.Fatalf("len=%d err=%v", len(b), err)
	}
	if _, err := download(context.Background(), ts.Client(), strings.Replace(ts.URL, "https", "http", 1), 4096); err == nil {
		t.Fatal("http url: no error")
	}
}

func goosForTest() string { return runtime.GOOS }
