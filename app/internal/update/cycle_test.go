package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/oLdpZ/diario-gilda/app/internal/addoninstall"
)

// Prova del ciclo di aggiornamento (Task 11, passo 3) senza GitHub: un server
// TLS con la forma dell'API pubblica prima v0.1.0 e poi v0.1.1, con zip fatti
// come nel workflow di release (VERSION in cima, firma dei byte dello zip).
// L'app 0.1.0 non si aggiorna su v0.1.0, si aggiorna su v0.1.1, poi al
// "riavvio" reinstalla l'addon 0.1.1 e non chiede altri aggiornamenti.
// La compatibilita' tra le chiavi di "firma keygen/sign" e Verify e' provata in
// app/cmd/firma (TestKeygenSignVerify).

type fakeGitHub struct {
	mu    sync.Mutex
	tag   string
	files map[string][]byte
	ts    *httptest.Server
}

func (g *fakeGitHub) publish(tag string, files map[string][]byte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tag, g.files = tag, files
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	g := &fakeGitHub{}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/oLdpZ/diario-gilda/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.tag == "" {
			http.NotFound(w, r)
			return
		}
		var as []asset
		for name := range g.files {
			as = append(as, asset{name, g.ts.URL + "/download/" + g.tag + "/" + name})
		}
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": g.tag, "draft": false, "prerelease": false, "assets": as,
		})
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		rest := strings.TrimPrefix(r.URL.Path, "/download/")
		tag, name, _ := strings.Cut(rest, "/")
		b, ok := g.files[name]
		if tag != g.tag || !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	g.ts = httptest.NewTLSServer(mux)
	t.Cleanup(g.ts.Close)
	return g
}

func addonFS(version string) fstest.MapFS {
	return fstest.MapFS{
		"DiarioGilda/DiarioGilda.toc": {Data: []byte("## Title: Diario di Gilda\n## Version: " + version + "\n\nMain.lua\n")},
		"DiarioGilda/Main.lua":        {Data: []byte("-- addon " + version + "\n")},
	}
}

func TestReleaseCycle(t *testing.T) {
	pub, priv := keys(t)
	const repo = "oLdpZ/diario-gilda"

	// release costruisce gli asset di una versione come fa release.yml.
	release := func(t *testing.T, ver string) map[string][]byte {
		win := makeZip(t, map[string]string{"VERSION": ver + "\n", exeWindows: "app " + ver})
		mac := makeZip(t, map[string]string{
			"VERSION":                          ver + "\n",
			exeMac:                             "app " + ver,
			bundleMac + "/Contents/Info.plist": "plist " + ver,
		})
		return map[string][]byte{
			assetWindows: win, assetWindows + ".sig": sign(priv, win),
			assetMac: mac, assetMac + ".sig": sign(priv, mac),
		}
	}

	for _, goos := range []string{"windows", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			gh := newFakeGitHub(t)
			ctx := context.Background()
			client := gh.ts.Client()

			var exe string
			if goos == "windows" {
				exe = filepath.Join(t.TempDir(), exeWindows)
				fakeExe(t, exe, "app 0.1.0")
			} else {
				exe = filepath.Join(t.TempDir(), "Applications", filepath.FromSlash(exeMac))
				fakeExe(t, exe, "app 0.1.0")
			}
			addons := t.TempDir()
			if err := addoninstall.Install(addonFS("0.1.0"), addons); err != nil {
				t.Fatal(err)
			}

			// Nessuna release ancora pubblicata.
			if rel, err := check(ctx, client, gh.ts.URL, repo, "0.1.0", goos, pub); rel != nil || err != nil {
				t.Fatalf("no release: rel=%v err=%v", rel, err)
			}

			// v0.1.0 pubblicata: l'app 0.1.0 non si aggiorna.
			gh.publish("v0.1.0", release(t, "0.1.0"))
			if rel, err := check(ctx, client, gh.ts.URL, repo, "0.1.0", goos, pub); rel != nil || err != nil {
				t.Fatalf("same version: rel=%v err=%v", rel, err)
			}

			// v0.1.1 pubblicata: si aggiorna.
			gh.publish("v0.1.1", release(t, "0.1.1"))
			rel, err := check(ctx, client, gh.ts.URL, repo, "0.1.0", goos, pub)
			if err != nil || rel == nil || rel.Version != "0.1.1" {
				t.Fatalf("check 0.1.1: rel=%+v err=%v", rel, err)
			}
			if err := install(ctx, client, rel, "0.1.0", pub, goos, exe); err != nil {
				t.Fatalf("install: %v", err)
			}
			if got := readFile(t, exe); got != "app 0.1.1" {
				t.Fatalf("exe after update = %q", got)
			}
			if goos == "darwin" {
				bundle, _ := bundleRoot(exe)
				if got := readFile(t, filepath.Join(bundle, "Contents", "Info.plist")); got != "plist 0.1.1" {
					t.Fatalf("Info.plist = %q", got)
				}
			}

			// "Riavvio" nella 0.1.1: pulizia della copia vecchia e addon aggiornato.
			cleanupOld(goos, exe)
			newAddon := addonFS("0.1.1")
			if !addoninstall.NeedsInstall(newAddon, addons) {
				t.Fatal("addon 0.1.0 installed but NeedsInstall is false")
			}
			if err := addoninstall.Install(newAddon, addons); err != nil {
				t.Fatal(err)
			}
			if got := addoninstall.Installed(addons); got != "0.1.1" {
				t.Fatalf("addon installed = %q", got)
			}

			// La 0.1.1 non si riaggiorna sulla stessa release.
			if rel, err := check(ctx, client, gh.ts.URL, repo, "0.1.1", goos, pub); rel != nil || err != nil {
				t.Fatalf("after update: rel=%v err=%v", rel, err)
			}
		})
	}
}
