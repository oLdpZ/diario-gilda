// Package discord invia file a un webhook Discord.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"time"
)

const requestTimeout = 60 * time.Second

// RetryAfter is returned on HTTP 429.
type RetryAfter struct{ Seconds float64 }

func (r *RetryAfter) Error() string {
	return fmt.Sprintf("discord: troppe richieste, riprova tra %.1f s", r.Seconds)
}

// HTTPError is a non-2xx, non-429 answer from Discord.
type HTTPError struct {
	Status int
	Msg    string // body, at most 200 characters
}

func (e *HTTPError) Error() string { return fmt.Sprintf("discord: HTTP %d: %s", e.Status, e.Msg) }

// Permanent reports whether err is a rejection that retrying the same file
// will not fix: any 4xx except 401/403/404 (webhook revoked or wrong: the file
// is fine and a new webhook will take it) and 429 (rate limit).
func Permanent(err error) bool {
	var he *HTTPError
	if !errors.As(err, &he) || he.Status < 400 || he.Status >= 500 {
		return false
	}
	switch he.Status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests:
		return false
	}
	return true
}

// Upload posts data as files[0] with a payload_json {"content": content}.
// The webhook URL is a secret and never appears in returned errors.
func Upload(ctx context.Context, webhookURL, filename string, data []byte, content string) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	payload, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return fmt.Errorf("discord: payload: %w", err)
	}
	if err := mw.WriteField("payload_json", string(payload)); err != nil {
		return fmt.Errorf("discord: multipart: %w", err)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files[0]"; filename=%q`, filename))
	h.Set("Content-Type", "application/gzip")
	part, err := mw.CreatePart(h)
	if err != nil {
		return fmt.Errorf("discord: multipart: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("discord: multipart: %w", err)
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("discord: multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, &body)
	if err != nil {
		return errors.New("discord: URL del webhook non valido")
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // drop the URL
		}
		return fmt.Errorf("discord: invio fallito: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests:
		var j struct {
			RetryAfter *float64 `json:"retry_after"`
		}
		if json.Unmarshal(raw, &j) == nil && j.RetryAfter != nil {
			return &RetryAfter{Seconds: *j.RetryAfter}
		}
		if s, err := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64); err == nil {
			return &RetryAfter{Seconds: s}
		}
		return &RetryAfter{}
	}
	msg := string(raw)
	if r := []rune(msg); len(r) > 200 {
		msg = string(r[:200])
	}
	return &HTTPError{Status: resp.StatusCode, Msg: msg}
}
