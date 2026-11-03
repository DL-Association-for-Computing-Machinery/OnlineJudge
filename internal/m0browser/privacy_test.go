package m0browser

import (
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func TestStaticPageAndBootstrapAreReadOnly(t *testing.T) {
	h, store, nonce := fixture(t)
	for _, path := range []string{"/m0", "/m0/", "/m0/assets/page.css", "/m0/assets/page.js", "/m0/api/bootstrap"} {
		w := call(t, h, "GET", path, "", "", nil)
		if w.Code != 200 || w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatal("page policy or static resource unavailable")
		}
		if path != "/m0/api/bootstrap" && strings.Contains(w.Body.String(), nonce) {
			t.Fatal("nonce leaked into static page")
		}
		if strings.Contains(w.Body.String(), strings.Repeat("private-fixture-", 3)) {
			t.Fatal("Bearer leaked into browser response")
		}
	}
	w := call(t, h, "HEAD", "/m0", "", "", nil)
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") == "" {
		t.Fatal("HEAD wrote page content or lost size metadata")
	}
	for _, path := range []string{"/m0/assets/../handler.go", "/m0/assets/unknown.js", "/m0/api/bootstrap?nonce=forbidden"} {
		w := call(t, h, "GET", path, "", "", nil)
		if w.Code != 403 && w.Code != 404 {
			t.Fatal("browser exposed an unlisted path")
		}
	}
	if store.reads != 0 || store.writes != 0 {
		t.Fatal("page GET/HEAD/bootstrap accessed submission data")
	}
}

func TestBrowserNonceRotatesAndDefaultRoutesStayClosed(t *testing.T) {
	h, store, nonce := fixture(t)
	other, err := NewHandler(httpapi.NewHandler(), store, testHost)
	if err != nil {
		t.Fatal(err)
	}
	if w := call(t, other, "GET", "/m0/api/receipt", "", nonce, nil); w.Code != 403 || store.reads != 0 {
		t.Fatal("old process nonce remained valid")
	}
	if w := call(t, httpapi.NewHandler(), "GET", "/m0", "", nonce, nil); w.Code != 404 {
		t.Fatal("default API exposed browser page")
	}
	if w := call(t, h, "GET", "/api/v1/healthz", "", "", nil); w.Code != 200 {
		t.Fatal("browser changed existing health route")
	}
	for _, addr := range []string{"", "127.0.0.1:0", "localhost:12345", "0.0.0.0:12345"} {
		if _, err := NewHandler(httpapi.NewHandler(), store, addr); !errors.Is(err, ErrConfiguration) {
			t.Fatal("constructor accepted invalid actual listener")
		}
	}
	if _, err := NewHandler(nil, store, testHost); !errors.Is(err, ErrConfiguration) {
		t.Fatal("constructor accepted nil base")
	}
	if _, err := NewHandler(httpapi.NewHandler(), nil, testHost); !errors.Is(err, ErrConfiguration) {
		t.Fatal("constructor accepted nil store")
	}
}

func TestBrowserStoreFailureRemainsUnknownAndPrivate(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{submission.ErrInvalid, 400, "INVALID_SUBMISSION"},
		{submission.ErrConflict, 409, "IDEMPOTENCY_CONFLICT"},
		{submission.ErrNotFound, 404, "NOT_FOUND"},
		{submission.ErrMaterial, 503, "MATERIAL_UNAVAILABLE"},
		{errors.New("synthetic internal sensitive marker"), 503, "ACCEPTANCE_UNKNOWN"},
	} {
		h, store, nonce := fixture(t)
		store.err = tc.err
		for _, method := range []string{"GET", "POST"} {
			path, body := "/m0/api/receipt", ""
			if method == "POST" {
				path, body = "/m0/api/submissions", `{"preset":"sum"}`
			}
			w := call(t, h, method, path, body, nonce, nil)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "sensitive marker") || len(store.values) != 0 {
				t.Fatal("error response changed unknown/private semantics")
			}
		}
	}
}
