package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadOK(t *testing.T) {
	var gotName, gotContent, gotCT string
	var gotData []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		var p struct{ Content string }
		_ = json.Unmarshal([]byte(r.FormValue("payload_json")), &p)
		gotContent = p.Content
		f, fh, err := r.FormFile("files[0]")
		if err != nil {
			t.Error(err)
			return
		}
		gotName = fh.Filename
		gotCT = fh.Header.Get("Content-Type")
		gotData, _ = io.ReadAll(f)
		w.WriteHeader(204)
	}))
	defer srv.Close()
	if err := Upload(context.Background(), srv.URL, "s.json.gz", []byte("abc"), "ciao"); err != nil {
		t.Fatal(err)
	}
	if gotName != "s.json.gz" || string(gotData) != "abc" || gotContent != "ciao" || gotCT != "application/gzip" {
		t.Fatalf("got %q %q %q %q", gotName, gotData, gotContent, gotCT)
	}
}

func TestUpload429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"retry_after": 1.5}`))
	}))
	defer srv.Close()
	err := Upload(context.Background(), srv.URL, "a", nil, "")
	ra, ok := err.(*RetryAfter)
	if !ok || ra.Seconds != 1.5 {
		t.Fatalf("err = %#v", err)
	}
}

func TestUpload429HeaderFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(429)
	}))
	defer srv.Close()
	err := Upload(context.Background(), srv.URL, "a", nil, "")
	ra, ok := err.(*RetryAfter)
	if !ok || ra.Seconds != 3 {
		t.Fatalf("err = %#v", err)
	}
}

func TestUpload500Truncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(strings.Repeat("x", 500)))
	}))
	defer srv.Close()
	err := Upload(context.Background(), srv.URL, "a", nil, "")
	if err == nil || !strings.Contains(err.Error(), "500") || len(err.Error()) > 300 {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadErrorRedactsURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL + "/api/webhooks/123/SECRET"
	srv.Close()
	err := Upload(context.Background(), url, "a", nil, "")
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("err = %v", err)
	}
}

func TestPermanent(t *testing.T) {
	for _, tc := range []struct {
		code int
		want bool
	}{
		{400, true}, {413, true}, {422, true},
		{401, false}, {403, false}, {404, false}, {429, false}, {500, false}, {502, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.code)
			_, _ = w.Write([]byte(`{"message":"no"}`))
		}))
		err := Upload(context.Background(), srv.URL, "a", nil, "")
		srv.Close()
		if got := Permanent(err); got != tc.want {
			t.Errorf("HTTP %d: Permanent = %v (err %v)", tc.code, got, err)
		}
		if err == nil || (tc.code != 429 && !strings.Contains(err.Error(), fmt.Sprint(tc.code))) {
			t.Errorf("HTTP %d: err = %v", tc.code, err)
		}
	}
	if Permanent(nil) || Permanent(errors.New("rete assente")) {
		t.Error("errori non HTTP permanenti")
	}
	if !Permanent(fmt.Errorf("outbox: invio: %w", &HTTPError{Status: 400})) {
		t.Error("errore avvolto non riconosciuto")
	}
}
