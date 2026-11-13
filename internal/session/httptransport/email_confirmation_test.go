package httptransport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
)

func TestEmailConfirmationHTTP(t *testing.T) {
	store := &fakeStore{}
	base := newHandler(t, store)
	const activation, reset = "/api/v1/account/activate", "/api/v1/account/reset-password"
	const body = `{"token":"synthetic-fixture-token","new_password":"  synthetic password  "}`
	calls := 0
	wantPurpose := emailtoken.Activation
	outcome := transport.EmailConfirmationCommitted
	var cancelDuringConfirmation context.CancelFunc
	var consumedPassword []byte
	h, err := transport.WithEmailConfirmation(base, func(ctx context.Context, source netip.Addr, purpose emailtoken.Purpose, value string, password []byte) transport.EmailConfirmationResult {
		calls++
		consumedPassword = password
		if _, ok := ctx.Deadline(); !ok || source.String() != "192.0.2.1" || purpose != wantPurpose ||
			value != "synthetic-fixture-token" || string(password) != "  synthetic password  " {
			t.Error("confirmation did not preserve trusted source, purpose or input bytes")
		}
		if cancelDuringConfirmation != nil {
			cancelDuringConfirmation()
		}
		return outcome
	})
	if err != nil {
		t.Fatal(err)
	}
	post := func(path, payload string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, origin+path, strings.NewReader(payload))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		return r
	}
	check := func(h http.Handler, r *http.Request, status int, code string) {
		t.Helper()
		w := response(t, h, r, status, false)
		if (code != "" && !strings.Contains(w.Body.String(), code)) ||
			(status == 204 && w.Body.Len() != 0) ||
			w.Header().Get("Referrer-Policy") != "no-referrer" ||
			strings.Contains(w.Body.String(), "synthetic") {
			t.Fatal("confirmation response was not empty or safely classified")
		}
	}
	check(base, post(activation, body), 404, "NOT_FOUND")
	check(h, httptest.NewRequest(http.MethodGet, origin+activation, nil), 405, "METHOD_NOT_ALLOWED")
	check(h, post(activation+"?token=fixture", body), 400, "INVALID_EMAIL_CONFIRMATION")
	check(h, post("/api/v1/account/%61ctivate", body), 400, "INVALID_EMAIL_CONFIRMATION")
	r := post(activation, body)
	r.Header.Set("Origin", "https://other.example.invalid")
	check(h, r, 403, "ORIGIN_REJECTED")
	r = post(activation, body)
	r.Header.Del("Origin")
	check(h, r, 403, "ORIGIN_REJECTED")
	r = post(activation, body)
	r.TLS = nil
	check(h, r, 403, "ORIGIN_REJECTED")
	r = post(activation, body)
	r.Header.Set("Content-Type", "text/plain")
	check(h, r, 415, "UNSUPPORTED_MEDIA_TYPE")
	r = post(activation, body)
	r.Header.Add("Content-Type", "application/json")
	check(h, r, 415, "UNSUPPORTED_MEDIA_TYPE")
	check(h, post(activation, strings.Repeat("x", 4097)), 413, "BODY_TOO_LARGE")
	for _, invalid := range []string{
		`{"token":"a","token":"b","new_password":"c"}`,
		`{"token":"a","new_password":null}`,
		`{"token":"a","new_password":"\uD800"}`,
		`{"token":"a","new_password":"b","subject_id":"c"}`,
		body + `{}`,
	} {
		check(h, post(activation, invalid), 400, "INVALID_EMAIL_CONFIRMATION")
	}
	if calls != 0 {
		t.Fatal("a refused request reached the confirmation service")
	}
	check(h, post(activation, body), 204, "")
	wantPurpose = emailtoken.Recovery
	for _, tc := range []struct {
		result transport.EmailConfirmationResult
		status int
		code   string
	}{
		{transport.EmailConfirmationCommitted, 204, ""},
		{transport.EmailConfirmationInvalidLink, 400, "EMAIL_LINK_UNAVAILABLE"},
		{transport.EmailConfirmationRateLimited, 429, "EMAIL_CONFIRMATION_RATE_LIMITED"},
		{transport.EmailConfirmationWeakPassword, 400, "WEAK_PASSWORD"},
		{transport.EmailConfirmationInvalidPassword, 400, "INVALID_NEW_PASSWORD"},
		{transport.EmailConfirmationUnavailable, 503, "EMAIL_CONFIRMATION_UNAVAILABLE"},
	} {
		outcome = tc.result
		check(h, post(reset, body), tc.status, tc.code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelDuringConfirmation = cancel
	outcome = transport.EmailConfirmationCommitted
	check(h, post(reset, body).WithContext(ctx), 503, "EMAIL_CONFIRMATION_UNAVAILABLE")
	for _, b := range consumedPassword {
		if b != 0 {
			t.Fatal("password buffer was not cleared")
		}
	}
	if calls != 8 || len(store.calls) != 0 {
		t.Fatal("confirmation retried a write or required an existing session")
	}
}
