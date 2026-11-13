package httptransport_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
)

func TestEmailRequestHTTP(t *testing.T) {
	store := &fakeStore{}
	base := newHandler(t, store)
	const register = "/api/v1/account/register"
	const reset = "/api/v1/account/request-password-reset"
	const payload = `{"email":"Fixture+Tag@Example.invalid"}`
	post := func(path, body string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, origin+path, strings.NewReader(body))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		return r
	}
	calls := 0
	var unavailable bool
	var cancelAfterIntake context.CancelFunc
	h, err := transport.WithEmailRequest(base, func(ctx context.Context, source netip.Addr, purpose emailtoken.Purpose, email identity.Email) error {
		calls++
		_, deadline := ctx.Deadline()
		if !deadline || source.String() != "192.0.2.1" || email.Original() != "Fixture+Tag@Example.invalid" || email.Key() != "fixture+tag@example.invalid" {
			t.Error("incorrect transport source or email")
		}
		if calls == 1 && purpose != emailtoken.Activation || calls > 1 && purpose != emailtoken.Recovery {
			t.Error("incorrect request purpose")
		}
		if unavailable {
			return errors.New("sensitive fixture detail")
		}
		if cancelAfterIntake != nil {
			cancelAfterIntake()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	response(t, base, post(register, payload), 404, false)
	response(t, h, httptest.NewRequest(http.MethodGet, origin+register, nil), 405, false)
	response(t, h, post(register+"?email=fixture", payload), 400, false)
	response(t, h, post(register, strings.Repeat("x", 2049)), 413, false)
	for _, body := range []string{`{"email":null}`, `{"email":"bad"}`, `{"Email":"Fixture+Tag@Example.invalid"}`,
		`{"email":"a@example.invalid","email":"b@example.invalid"}`, payload + `{}`, `{"email":"a@example.invalid","password":"ignored"}`} {
		response(t, h, post(register, body), 400, false)
	}
	r := post(register, payload)
	r.Header.Del("Origin")
	response(t, h, r, 403, false)
	r = post(register, payload)
	r.Header.Add("Content-Type", "application/json")
	response(t, h, r, 415, false)
	if calls != 0 {
		t.Fatal("a refused request reached the service")
	}
	for _, path := range []string{register, reset} {
		if w := response(t, h, post(path, payload), 202, false); w.Body.Len() != 0 || w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("public acceptance exposed a receipt or omitted referrer protection")
		}
	}
	unavailable = true
	w := response(t, h, post(reset, payload), 503, false)
	if strings.Contains(w.Body.String(), "sensitive fixture") || calls != 3 || len(store.calls) != 0 {
		t.Fatal("unavailable exposed diagnostics, retried, or required an existing session")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	unavailable, cancelAfterIntake = false, cancel
	response(t, h, post(reset, payload).WithContext(ctx), 202, false)
	if calls != 4 {
		t.Fatal("confirmed intake was repeated after cancellation")
	}
}
