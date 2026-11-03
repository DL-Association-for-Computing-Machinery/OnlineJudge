package m0browser

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

const testHost = "127.0.0.1:23456"

type savedOperation struct {
	request submission.Request
	receipt submission.Receipt
}

type observedStore struct {
	values map[string]savedOperation
	reads  int
	writes int
	err    error
}

func operationID(subject, scope, key string) string { return subject + ":" + scope + ":" + key }

func (s *observedStore) Accept(_ context.Context, subject, scope, key string, req submission.Request) (submission.Receipt, bool, error) {
	s.writes++
	if s.err != nil {
		return submission.Receipt{}, false, s.err
	}
	id := operationID(subject, scope, key)
	if old, exists := s.values[id]; exists {
		if old.request != req {
			return submission.Receipt{}, false, submission.ErrConflict
		}
		return old.receipt, false, nil
	}
	r := submission.Receipt{ID: strings.Repeat("a", 32), MaterialVersion: req.MaterialVersion, Language: req.Language, CreatedAt: time.Now().UTC(), State: "queued"}
	s.values[id] = savedOperation{req, r}
	return r, true, nil
}

func (s *observedStore) Receipt(_ context.Context, subject, scope, key string) (submission.Receipt, error) {
	s.reads++
	if s.err != nil {
		return submission.Receipt{}, s.err
	}
	if value, ok := s.values[operationID(subject, scope, key)]; ok {
		return value.receipt, nil
	}
	return submission.Receipt{}, submission.ErrNotFound
}

func (*observedStore) Get(context.Context, string, string, string) (submission.Receipt, error) {
	return submission.Receipt{}, errors.New("browser must not use another lookup scope")
}

func fixture(t *testing.T) (http.Handler, *observedStore, string) {
	t.Helper()
	store := &observedStore{values: make(map[string]savedOperation)}
	base := httpapi.NewHandler(httpapi.Experiment{Store: store, Token: strings.Repeat("private-fixture-", 3)})
	h, err := NewHandler(base, store, testHost)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/m0/api/bootstrap", nil)
	r.Host = testHost
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var data struct{ Nonce string }
	if json.Unmarshal(w.Body.Bytes(), &data) != nil || len(data.Nonce) != 64 || w.Code != 200 {
		t.Fatal("bootstrap unavailable")
	}
	return h, store, data.Nonce
}

func call(t *testing.T, h http.Handler, method, path, body, nonce string, alter func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = testHost
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", "m0-browser-operation-key")
	r.Header.Set(nonceHeader, nonce)
	if method == http.MethodPost {
		r.Header.Set("Origin", "http://"+testHost)
		r.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	if alter != nil {
		alter(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Access-Control-Allow-Origin") != "" || len(w.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("local privacy boundary missing")
	}
	return w
}

func TestFixedPresetAcceptanceAndScope(t *testing.T) {
	h, store, nonce := fixture(t)
	key := "m0-browser-operation-key"
	for _, status := range []int{202, 200} {
		w := call(t, h, "POST", "/m0/api/submissions", `{"preset":"sum"}`, nonce, nil)
		if w.Code != status {
			t.Fatalf("acceptance status=%d want=%d", w.Code, status)
		}
	}
	stored := store.values[operationID(subject, scope, key)]
	if len(store.values) != 1 || stored.request.Source != presets[0].Source || stored.request.Language != "gnu++17" || stored.request.MaterialVersion != "m0-sum-v3" {
		t.Fatal("browser changed fixed source or attribution")
	}
	if w := call(t, h, "POST", "/m0/api/submissions", `{"preset":"wrong"}`, nonce, nil); w.Code != 409 || store.values[operationID(subject, scope, key)] != stored {
		t.Fatal("conflict changed original operation")
	}
	if w := call(t, h, "GET", "/m0/api/receipt", "", nonce, nil); w.Code != 200 {
		t.Fatal("original-key receipt not available")
	}
	// Same key in the Bearer scope must not become visible to browser lookup.
	delete(store.values, operationID(subject, scope, key))
	store.values[operationID("m0-operator", "m0-training", key)] = stored
	if w := call(t, h, "GET", "/m0/api/receipt", "", nonce, nil); w.Code != 404 {
		t.Fatal("browser crossed Bearer submission scope")
	}
	// The existing API still enforces its own token and Origin restrictions.
	w := call(t, h, "POST", "/api/v1/experiments/submissions", `{}`, nonce, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("private-fixture-", 3))
	})
	if w.Code != 403 {
		t.Fatal("browser bypassed existing Bearer Origin rejection")
	}
}

func TestBrowserRejectionsNeverCallStore(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
		status                   int
		alter                    func(*http.Request)
	}{
		{"wrong host", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 403, func(r *http.Request) { r.Host = "example.invalid" }},
		{"missing nonce", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 403, func(r *http.Request) { r.Header.Del(nonceHeader) }},
		{"wrong nonce", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 403, func(r *http.Request) { r.Header.Set(nonceHeader, "wrong") }},
		{"duplicate nonce", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 403, func(r *http.Request) { r.Header.Add(nonceHeader, "extra") }},
		{"missing Origin", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 403, func(r *http.Request) { r.Header.Del("Origin") }},
		{"duplicate Origin", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 403, func(r *http.Request) { r.Header.Add("Origin", "http://"+testHost) }},
		{"cross-site Origin", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 403, func(r *http.Request) { r.Header.Set("Origin", "https://example.invalid") }},
		{"cross-site metadata", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 403, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }},
		{"duplicate metadata", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 403, func(r *http.Request) { r.Header.Add("Sec-Fetch-Site", "same-origin") }},
		{"query rejected", "POST", "/m0/api/submissions?nonce=extra", `{"preset":"sum"}`, 403, nil},
		{"absolute form", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 403, func(r *http.Request) { r.URL.Host = testHost }},
		{"GET requires nonce", "GET", "/m0/api/receipt", "", 403, func(r *http.Request) { r.Header.Del(nonceHeader) }},
		{"GET foreign Origin", "GET", "/m0/api/receipt", "", 403, func(r *http.Request) { r.Header.Set("Origin", "https://example.invalid") }},
		{"HEAD no action", "HEAD", "/m0/api/submissions", "", 405, nil},
		{"OPTIONS no action", "OPTIONS", "/m0/api/submissions", "", 405, nil},
		{"wrong key", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 400, func(r *http.Request) { r.Header.Set("Idempotency-Key", "short") }},
		{"duplicate key", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 400, func(r *http.Request) { r.Header.Add("Idempotency-Key", "another-operation-key") }},
		{"source injection", "POST", "/m0/api/submissions", `{"preset":"sum","source":"arbitrary"}`, 400, nil},
		{"subject injection", "POST", "/m0/api/submissions", `{"preset":"sum","subject":"other"}`, 400, nil},
		{"context injection", "POST", "/m0/api/submissions", `{"preset":"sum","context":"other"}`, 400, nil},
		{"unknown preset", "POST", "/m0/api/submissions", `{"preset":"other"}`, 400, nil},
		{"duplicate field", "POST", "/m0/api/submissions", `{"preset":"sum","preset":"wrong"}`, 400, nil},
		{"case folded field", "POST", "/m0/api/submissions", `{"Preset":"sum"}`, 400, nil},
		{"malformed JSON", "POST", "/m0/api/submissions", `{`, 400, nil},
		{"null", "POST", "/m0/api/submissions", `null`, 400, nil},
		{"empty", "POST", "/m0/api/submissions", `{}`, 400, nil},
		{"array", "POST", "/m0/api/submissions", `[]`, 400, nil},
		{"unpaired Unicode", "POST", "/m0/api/submissions", `{"preset":"\ud800"}`, 400, nil},
		{"invalid UTF8", "POST", "/m0/api/submissions", string([]byte{0xff}), 400, nil},
		{"two objects", "POST", "/m0/api/submissions", `{"preset":"sum"}{}`, 400, nil},
		{"over bound", "POST", "/m0/api/submissions", strings.Repeat(" ", 513), 413, nil},
		{"wrong media type", "POST", "/m0/api/submissions", `{"preset":"sum"}`, 415, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, store, nonce := fixture(t)
			w := call(t, h, tc.method, tc.path, tc.body, nonce, tc.alter)
			if w.Code != tc.status || store.reads != 0 || store.writes != 0 || len(store.values) != 0 {
				t.Fatalf("rejection status=%d want=%d reads=%d writes=%d", w.Code, tc.status, store.reads, store.writes)
			}
		})
	}
}
