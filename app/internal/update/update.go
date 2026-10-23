// Package update cerca una versione nuova su GitHub Releases, ne verifica la
// firma ed25519 e sostituisce l'eseguibile.
//
// Regole di sicurezza:
//   - lo zip si verifica PRIMA di estrarlo o applicarlo; senza chiave pubblica
//     (build di sviluppo) Check non segnala mai aggiornamenti e Install rifiuta;
//   - la firma copre i byte esatti dello zip scaricato; il tag della release non
//     e' firmato, quindi ogni zip contiene un file VERSION in cima (es. "0.2.0\n"),
//     che dopo la verifica deve essere uguale a Release.Version e piu' nuovo della
//     versione attuale: cosi' un tag falso non puo' riproporre uno zip vecchio;
//   - anche i redirect devono restare in https;
//   - si seguono solo URL https (anche apiBase): i test usano httptest.NewTLSServer
//     con il suo client, senza eccezioni per l'http in chiaro;
//   - download con limite di dimensione, estrazione con limite e senza percorsi
//     assoluti o "..", senza link simbolici (il bundle macOS prodotto dal workflow
//     di release non ne contiene), solo le voci attese.
//
// Windows: l'eseguibile si sostituisce con selfupdate (il vecchio resta in
// <exe>.old). macOS: il bundle intero viene preparato in DiarioGilda.app.new
// accanto a quello in uso e scambiato con due rename (il vecchio resta in
// DiarioGilda.app.old); a un errore si rimette quello vecchio.
//
// Versioni: MAJOR.MINOR.PATCH numerico, "v" iniziale ignorata. Un tag con suffisso
// di pre-release ("0.1.1-test") si salta, a meno che anche la versione attuale ne
// abbia uno; a parita' di numeri la release senza suffisso e' piu' nuova di quella
// con suffisso, e due pre-release con gli stessi numeri non si aggiornano a vicenda.
package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/minio/selfupdate"

	"github.com/oLdpZ/diario-gilda/app/internal/build"
)

const (
	assetWindows = "DiarioGilda-windows-amd64.zip"
	assetMac     = "DiarioGilda-macos.zip"

	exeWindows  = "DiarioGilda.exe"
	bundleMac   = "DiarioGilda.app"
	exeMac      = bundleMac + "/Contents/MacOS/DiarioGilda"
	versionFile = "VERSION"

	maxAPI      = 1 << 20   // risposta JSON dell'API
	maxSig      = 4 << 10   // file .sig
	maxDownload = 100 << 20 // zip scaricato
	maxUnzipped = 150 << 20 // somma dei file estratti

	defaultTimeout = 5 * time.Minute
)

// Release e' una versione piu' nuova disponibile per questa piattaforma.
type Release struct {
	Version  string // senza "v"
	AssetURL string
	SigURL   string
}

// Check chiede a {apiBase}/repos/{repo}/releases/latest l'ultima release e la
// restituisce se e' piu' nuova di current; nil se non lo e', se current e' "dev"
// (o non e' una versione), se la build non ha build.PublicKey o se il repository
// non ha release. /releases/latest non restituisce mai pre-release ne' bozze; i
// controlli su prerelease/draft e sul suffisso del tag sono una difesa in piu'.
// client dovrebbe avere un Timeout; se nil se ne usa uno con 5 minuti.
func Check(ctx context.Context, client *http.Client, apiBase, repo, current string) (*Release, error) {
	return check(ctx, client, apiBase, repo, current, runtime.GOOS, build.PublicKey)
}

func check(ctx context.Context, client *http.Client, apiBase, repo, current, goos, pubKeyB64 string) (*Release, error) {
	if pubKeyB64 == "" {
		return nil, nil // senza chiave non si potrebbe verificare: niente aggiornamenti
	}
	cur, err := parseVersion(current)
	if err != nil {
		return nil, nil // "dev" o build senza versione: mai aggiornare
	}
	name, err := assetName(goos)
	if err != nil {
		return nil, err
	}
	u := strings.TrimRight(apiBase, "/") + "/repos/" + repo + "/releases/latest"
	if err := requireHTTPS(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("richiesta release: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "DiarioGilda/"+current)
	resp, err := httpClient(client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("richiesta release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // nessuna release pubblicata
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("richiesta release: HTTP %d", resp.StatusCode)
	}
	var body struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPI)).Decode(&body); err != nil {
		return nil, fmt.Errorf("risposta release: %w", err)
	}
	if body.Draft {
		return nil, nil
	}
	next, err := parseVersion(body.TagName)
	if err != nil {
		return nil, fmt.Errorf("tag della release %q: %w", body.TagName, err)
	}
	if (next.pre != "" || body.Prerelease) && cur.pre == "" {
		return nil, nil
	}
	if !next.newerThan(cur) {
		return nil, nil
	}
	rel := &Release{Version: strings.TrimPrefix(body.TagName, "v")}
	for _, a := range body.Assets {
		switch a.Name {
		case name:
			rel.AssetURL = a.URL
		case name + ".sig":
			rel.SigURL = a.URL
		}
	}
	if rel.AssetURL == "" || rel.SigURL == "" {
		return nil, fmt.Errorf("release %s senza %s o la sua firma", rel.Version, name)
	}
	for _, s := range []string{rel.AssetURL, rel.SigURL} {
		if err := requireHTTPS(s); err != nil {
			return nil, err
		}
	}
	return rel, nil
}

// Install scarica lo zip e la firma di rel, verifica la firma con build.PublicKey,
// controlla che il file VERSION dello zip sia rel.Version e piu' nuovo di current,
// e solo allora applica l'aggiornamento. Non riavvia il processo: la nuova
// versione parte al prossimo avvio (chi chiama decide quando).
func Install(ctx context.Context, client *http.Client, rel *Release, current string) error {
	exe, err := executable()
	if err != nil {
		return err
	}
	return install(ctx, client, rel, current, build.PublicKey, runtime.GOOS, exe)
}

func install(ctx context.Context, client *http.Client, rel *Release, current, pubKeyB64, goos, exe string) error {
	if pubKeyB64 == "" {
		return ErrNoKey
	}
	sig, err := download(ctx, client, rel.SigURL, maxSig)
	if err != nil {
		return fmt.Errorf("firma: %w", err)
	}
	data, err := download(ctx, client, rel.AssetURL, maxDownload)
	if err != nil {
		return fmt.Errorf("aggiornamento: %w", err)
	}
	if err := Verify(data, sig, pubKeyB64); err != nil {
		return err
	}
	files, err := unzip(data)
	if err != nil {
		return err
	}
	if err := checkVersionFile(files, rel.Version, current); err != nil {
		return err
	}
	return applyFiles(files, goos, exe, os.Rename)
}

// checkVersionFile: il VERSION firmato deve coincidere col tag ed essere piu' nuovo.
func checkVersionFile(files map[string]zfile, tag, current string) error {
	f, ok := files[versionFile]
	if !ok {
		return errors.New("lo zip non contiene VERSION: aggiornamento rifiutato")
	}
	got := strings.TrimPrefix(strings.TrimSpace(string(f.data)), "v")
	if got != strings.TrimPrefix(tag, "v") {
		return fmt.Errorf("lo zip e' la versione %q, non %q: aggiornamento rifiutato", got, tag)
	}
	next, err := parseVersion(got)
	if err != nil {
		return fmt.Errorf("VERSION dello zip: %w", err)
	}
	cur, err := parseVersion(current)
	if err != nil || !next.newerThan(cur) {
		return fmt.Errorf("la versione %s non e' piu' nuova di %q: aggiornamento rifiutato", got, current)
	}
	return nil
}

// ErrNoKey: build senza chiave pubblica, gli aggiornamenti sono disattivati.
var ErrNoKey = errors.New("chiave pubblica assente: aggiornamenti disattivati")

// Verify controlla che sig (base64, spazi e a-capo ammessi) sia la firma ed25519
// di data per la chiave pubblica pubKeyB64 (base64).
func Verify(data, sig []byte, pubKeyB64 string) error {
	if pubKeyB64 == "" {
		return ErrNoKey
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubKeyB64))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("chiave pubblica non valida")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(raw) != ed25519.SignatureSize {
		return errors.New("firma non valida")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), data, raw) {
		return errors.New("firma non corrispondente: aggiornamento rifiutato")
	}
	return nil
}

// CleanupOld cancella la copia di ripristino lasciata da un aggiornamento
// precedente (<exe>.old su Windows, dove si puo' solo dopo il riavvio;
// DiarioGilda.app.old su macOS). Errori ignorati.
func CleanupOld() {
	if exe, err := executable(); err == nil {
		cleanupOld(runtime.GOOS, exe)
	}
}

func cleanupOld(goos, exe string) {
	switch goos {
	case "darwin":
		if bundle, err := bundleRoot(exe); err == nil {
			os.RemoveAll(bundle + ".old")
		}
	default:
		os.Remove(exe + ".old")
	}
}

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("percorso dell'eseguibile: %w", err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe, nil
}

// apply applica uno zip gia' verificato, senza controllare VERSION (lo fa install).
func apply(zipData []byte, goos, exe string) error {
	files, err := unzip(zipData)
	if err != nil {
		return err
	}
	return applyFiles(files, goos, exe, os.Rename)
}

func applyFiles(files map[string]zfile, goos, exe string, rename func(string, string) error) error {
	switch goos {
	case "windows":
		bin, ok := files[exeWindows]
		if !ok || len(bin.data) == 0 {
			return fmt.Errorf("lo zip non contiene %s", exeWindows)
		}
		opts := selfupdate.Options{TargetPath: exe, OldSavePath: exe + ".old"}
		if err := selfupdate.Apply(bytes.NewReader(bin.data), opts); err != nil {
			if rerr := selfupdate.RollbackError(err); rerr != nil {
				return fmt.Errorf("aggiornamento fallito e ripristino fallito: %v (%w)", rerr, err)
			}
			return fmt.Errorf("aggiornamento fallito, versione precedente intatta: %w", err)
		}
		return nil
	case "darwin":
		return swapBundle(files, exe, rename)
	}
	return fmt.Errorf("piattaforma %s non supportata", goos)
}

// bundleRoot ricava .../X.app da .../X.app/Contents/MacOS/<eseguibile>.
func bundleRoot(exe string) (string, error) {
	macos := filepath.Dir(exe)
	contents := filepath.Dir(macos)
	bundle := filepath.Dir(contents)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(bundle, ".app") {
		return "", fmt.Errorf("%s non e' dentro un bundle .app", exe)
	}
	return bundle, nil
}

// swapBundle prepara il bundle dello zip in <bundle>.new, poi <bundle> -> <bundle>.old
// e <bundle>.new -> <bundle>; a un errore rimette il bundle vecchio.
func swapBundle(files map[string]zfile, exe string, rename func(string, string) error) error {
	if bin, ok := files[exeMac]; !ok || len(bin.data) == 0 {
		return fmt.Errorf("lo zip non contiene %s", exeMac)
	}
	bundle, err := bundleRoot(exe)
	if err != nil {
		return err
	}
	newB, oldB := bundle+".new", bundle+".old"
	if err := os.RemoveAll(newB); err != nil {
		return fmt.Errorf("pulizia di %s: %w", newB, err)
	}
	prefix := bundleMac + "/"
	for name, f := range files {
		rel, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue // voci fuori dal bundle (es. VERSION) non si estraggono
		}
		mode := f.mode
		if name == exeMac {
			mode |= 0o755
		}
		dst := filepath.Join(newB, filepath.FromSlash(rel))
		err := os.MkdirAll(filepath.Dir(dst), 0o755)
		if err == nil {
			err = os.WriteFile(dst, f.data, mode)
		}
		if err != nil {
			os.RemoveAll(newB)
			return fmt.Errorf("preparazione del bundle (%s): %w", name, err)
		}
	}
	if err := os.RemoveAll(oldB); err != nil {
		os.RemoveAll(newB)
		return fmt.Errorf("pulizia di %s: %w", oldB, err)
	}
	if err := rename(bundle, oldB); err != nil {
		os.RemoveAll(newB)
		return fmt.Errorf("spostamento del bundle attuale: %w", err)
	}
	if err := rename(newB, bundle); err != nil {
		if rerr := rename(oldB, bundle); rerr != nil {
			return fmt.Errorf("installazione del bundle: %w (ripristino fallito: %v)", err, rerr)
		}
		os.RemoveAll(newB)
		return fmt.Errorf("installazione del bundle, versione precedente intatta: %w", err)
	}
	return nil
}

type zfile struct {
	data []byte
	mode os.FileMode
}

// unzip legge in memoria i file normali dello zip, rifiutando percorsi pericolosi,
// voci duplicate, link simbolici e un totale estratto oltre maxUnzipped.
func unzip(zipData []byte) (map[string]zfile, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("zip non valido: %w", err)
	}
	files := map[string]zfile{}
	var total int64
	for _, f := range zr.File {
		name, err := safeName(f.Name)
		if err != nil {
			return nil, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if !f.Mode().IsRegular() {
			return nil, fmt.Errorf("zip: %s non e' un file normale", f.Name)
		}
		if _, dup := files[name]; dup {
			return nil, fmt.Errorf("zip: voce duplicata %s", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("zip: %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxUnzipped-total+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("zip: %s: %w", f.Name, err)
		}
		total += int64(len(data))
		if total > maxUnzipped {
			return nil, errors.New("zip: contenuto troppo grande")
		}
		mode := f.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		mode |= 0o600
		files[name] = zfile{data: data, mode: mode}
	}
	return files, nil
}

func safeName(name string) (string, error) {
	bad := name == "" || strings.ContainsAny(name, `\:`) || strings.HasPrefix(name, "/")
	clean := path.Clean(name)
	if !bad && (clean == ".." || strings.HasPrefix(clean, "../")) {
		bad = true
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			bad = true
		}
	}
	if bad {
		return "", fmt.Errorf("zip: percorso non ammesso %q", name)
	}
	return clean, nil
}

func download(ctx context.Context, client *http.Client, u string, limit int64) ([]byte, error) {
	if err := requireHTTPS(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "DiarioGilda")
	resp, err := httpClient(client).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("file troppo grande (%d byte)", resp.ContentLength)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("download interrotto: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file oltre %d byte", limit)
	}
	return data, nil
}

// httpClient copia il client (o ne crea uno con timeout) e rifiuta i redirect
// verso URL non https, mantenendo l'eventuale CheckRedirect del chiamante.
func httpClient(c *http.Client) *http.Client {
	var cp http.Client
	if c != nil {
		cp = *c
	} else {
		cp.Timeout = defaultTimeout
	}
	prev := cp.CheckRedirect
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("redirect non https rifiutato: %s", req.URL)
		}
		if prev != nil {
			return prev(req, via)
		}
		if len(via) >= 10 {
			return errors.New("troppi redirect")
		}
		return nil
	}
	return &cp
}

func requireHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("URL non https rifiutato: %q", raw)
	}
	return nil
}

func assetName(goos string) (string, error) {
	switch goos {
	case "windows":
		return assetWindows, nil
	case "darwin":
		return assetMac, nil
	}
	return "", fmt.Errorf("piattaforma %s non supportata", goos)
}

type version struct {
	num [3]int
	pre string
}

func parseVersion(s string) (version, error) {
	var v version
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	s, _, _ = strings.Cut(s, "+") // metadati di build ignorati
	s, v.pre, _ = strings.Cut(s, "-")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("versione %q non nel formato MAJOR.MINOR.PATCH", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || strings.Trim(p, "0123456789") != "" {
			return v, fmt.Errorf("versione %q non numerica", s)
		}
		v.num[i] = n
	}
	return v, nil
}

func (v version) newerThan(o version) bool {
	for i := range v.num {
		if v.num[i] != o.num[i] {
			return v.num[i] > o.num[i]
		}
	}
	return v.pre == "" && o.pre != ""
}
