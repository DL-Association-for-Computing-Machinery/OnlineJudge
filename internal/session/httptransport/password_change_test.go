package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func TestPasswordChangeBodyPreservesBothRawPasswords(t *testing.T) {
	for _, tc := range []struct{ name, body, media, current, replacement string }{
		{"raw Unicode and whitespace", `{"current_password":" old\\密\u0000码 ","new_password":" e\u0301😀\\ 三 "}`, "application/json", " old\\密\x00码 ", " e\u0301😀\\ 三 "},
		{"escaped keys", `{"current_pass\u0077ord":"a","new_pass\u0077ord":"b"}`, "application/json", "a", "b"},
		{"reordered and media parameter", `{"new_password":"b","current_password":"a"}`, "application/json; charset=utf-8", "a", "b"},
		{"paired surrogate", `{"current_password":"a","new_password":"\ud83d\ude00"}`, "application/json", "a", "😀"},
		{"current over hasher budget and valid replacement", `{"current_password":"` + strings.Repeat("c", 129) + `","new_password":"valid synthetic replacement"}`, "application/json", strings.Repeat("c", 129), "valid synthetic replacement"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "https://example.invalid", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			current, replacement, ok := passwordChangeBody(w, r)
			defer clear(current)
			defer clear(replacement)
			if !ok || string(current) != tc.current || string(replacement) != tc.replacement || w.Body.Len() != 0 || len(w.Header().Values("Set-Cookie")) != 0 {
				t.Fatal("password body parser changed raw input or published a response")
			}
		})
	}
	base := `{"current_password":"a","new_password":"b"}`
	for _, size := range []int{4096, 4097} {
		body := base + strings.Repeat(" ", size-len(base))
		r := httptest.NewRequest("POST", "https://example.invalid", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		current, replacement, ok := passwordChangeBody(w, r)
		clear(current)
		clear(replacement)
		if (size == 4096) != ok || (size == 4097 && (w.Code != 413 || current != nil || replacement != nil)) {
			t.Fatal("password body byte boundary changed")
		}
	}
}

func TestPasswordChangeBodyRejectsAmbiguousOrInvalidInputs(t *testing.T) {
	base := `{"current_password":"synthetic current","new_password":"synthetic replacement"}`
	for name, body := range map[string]string{
		"empty": "", "malformed": "{", "root null": "null", "array": "[]", "string": `"fixture"`,
		"missing current": `{"new_password":"synthetic replacement"}`, "missing new": `{"current_password":"synthetic current"}`,
		"empty current": `{"current_password":"","new_password":"x"}`, "empty new": `{"current_password":"x","new_password":""}`,
		"null current": `{"current_password":null,"new_password":"x"}`, "null new": `{"current_password":"x","new_password":null}`,
		"numeric current": `{"current_password":42,"new_password":"x"}`, "object new": `{"current_password":"x","new_password":{}}`,
		"unknown selector":          strings.Replace(base, `"current_password":`, `"subject_id":"untrusted","current_password":`, 1),
		"duplicate current":         strings.Replace(base, `"current_password":`, `"current_password":"other","current_password":`, 1),
		"escaped duplicate current": strings.Replace(base, `"current_password":`, `"current_pass\u0077ord":"other","current_password":`, 1),
		"escaped duplicate new":     strings.Replace(base, `"new_password":`, `"new_pass\u0077ord":"other","new_password":`, 1),
		"case alias":                strings.Replace(base, `"current_password"`, `"Current_Password"`, 1),
		"unpaired current":          `{"current_password":"\ud800","new_password":"x"}`, "unpaired new": `{"current_password":"x","new_password":"\udc00"}`,
		"invalid UTF8":    `{"current_password":"x","new_password":"` + string([]byte{0xff}) + `"}`,
		"trailing object": base + "{}", "trailing value": base + " true",
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "https://example.invalid", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			current, replacement, ok := passwordChangeBody(w, r)
			if ok || current != nil || replacement != nil || w.Code != 400 || len(w.Header().Values("Set-Cookie")) != 0 || strings.Contains(w.Body.String(), "synthetic") {
				t.Fatal("invalid body accepted, reclassified or reflected")
			}
		})
	}
	for _, media := range []string{"", "text/plain", "application/problem+json", `application/json; charset="unterminated`} {
		r := httptest.NewRequest("POST", "https://example.invalid", strings.NewReader(base))
		r.Header.Set("Content-Type", media)
		w := httptest.NewRecorder()
		a, b, ok := passwordChangeBody(w, r)
		if ok || a != nil || b != nil || w.Code != 415 {
			t.Fatal("unsupported media accepted")
		}
	}
	r := httptest.NewRequest("POST", "https://example.invalid", nil)
	r.Header.Set("Content-Type", "application/json")
	r.Body = io.NopCloser(passwordBodyFailure{})
	w := httptest.NewRecorder()
	a, b, ok := passwordChangeBody(w, r)
	if ok || a != nil || b != nil || w.Code != 400 || strings.Contains(w.Body.String(), "synthetic reader cause") {
		t.Fatal("body read error was accepted or reflected")
	}
}

type passwordBodyFailure struct{}

func (passwordBodyFailure) Read([]byte) (int, error) { return 0, errors.New("synthetic reader cause") }

func checkPasswordChangeProblem(t *testing.T, err error, status int, code string) {
	t.Helper()
	w := httptest.NewRecorder()
	passwordChangeError(w, err)
	var problem struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(w.Body.Bytes(), &problem) != nil || w.Code != status || problem.Code != code || len(w.Header().Values("Set-Cookie")) != 0 ||
		w.Header().Get("Retry-After") != "" || w.Header().Get("Access-Control-Allow-Origin") != "" || bytes.Contains(w.Body.Bytes(), []byte("synthetic")) {
		t.Fatal("password change error published cookies, causes or wrong status/code")
	}
}

func TestPasswordChangeErrorClassificationAndUnknownPriority(t *testing.T) {
	refused := []struct {
		err    error
		status int
		code   string
	}{
		{redisstore.ErrPasswordChangeRate, 429, "PASSWORD_CHANGE_RATE_LIMITED"},
		{password.ErrWeakPassword, 400, "WEAK_PASSWORD"}, {password.ErrNewPasswordLength, 400, "INVALID_NEW_PASSWORD"},
		{password.ErrPassword, 400, "INVALID_PASSWORD_INPUT"}, {identity.ErrPasswordUnchanged, 400, "PASSWORD_UNCHANGED"},
		{identity.ErrConflict, 409, "PASSWORD_CHANGE_CONFLICT"}, {identity.ErrCredentials, 401, "PASSWORD_CHANGE_REJECTED"},
		{redisstore.ErrStale, 401, "PASSWORD_CHANGE_REJECTED"}, {redisstore.ErrNotFound, 401, "PASSWORD_CHANGE_REJECTED"},
		{redisstore.ErrInvalid, 400, "INVALID_PASSWORD_CHANGE"},
	}
	for _, tc := range refused {
		checkPasswordChangeProblem(t, errors.Join(tc.err, errors.New("synthetic diagnostic")), tc.status, tc.code)
		for _, unknown := range []error{identity.ErrStore, redisstore.ErrStore, identity.ErrRecovering, redisstore.ErrUnknown, context.Canceled, context.DeadlineExceeded} {
			checkPasswordChangeProblem(t, errors.Join(tc.err, identity.ErrGuardedOperation, unknown, errors.New("synthetic diagnostic")), 503, "PASSWORD_CHANGE_UNAVAILABLE")
		}
	}
	checkPasswordChangeProblem(t, errors.New("synthetic unknown cause"), 503, "PASSWORD_CHANGE_UNAVAILABLE")
}

type passwordUnusedStore struct{ calls int }

func (s *passwordUnusedStore) Read(context.Context, redisstore.Credential) (redisstore.View, error) {
	s.calls++
	return redisstore.View{}, redisstore.ErrInvalid
}
func (s *passwordUnusedStore) Touch(context.Context, redisstore.Credential) (redisstore.View, error) {
	s.calls++
	return redisstore.View{}, redisstore.ErrInvalid
}
func (s *passwordUnusedStore) Revoke(context.Context, redisstore.Credential) error {
	s.calls++
	return redisstore.ErrInvalid
}

func TestPasswordChangeRequiresExplicitValidCapability(t *testing.T) {
	config := Config{Origin: "https://example.invalid", Timeout: time.Second}
	store := &passwordUnusedStore{}
	old, err := New(store, config)
	if err != nil {
		t.Fatal("existing session constructor failed")
	}
	withoutCapability := &handler{store: store, login: &redisstore.Login{}, config: config, host: "example.invalid"}
	for _, h := range []http.Handler{old, withoutCapability} {
		for _, method := range []string{"GET", "POST"} {
			r := httptest.NewRequest(method, config.Origin+"/api/v1/account/password", nil)
			r.Header.Set("Origin", config.Origin)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 404 || len(w.Header().Values("Set-Cookie")) != 0 || store.calls != 0 {
				t.Fatal("absent capability opened password-change path")
			}
		}
	}
	if h, err := NewLogin(nil, config); h != nil || err == nil {
		t.Fatal("nil login constructed password-change entry")
	}
	for _, capability := range []*redisstore.PasswordChange{nil, {}} {
		if h, err := NewLimitedLogin(nil, config, capability); h != nil || err == nil {
			t.Fatal("nil/zero capability enabled limited login")
		}
		if h, err := NewProxiedLimitedLogin(nil, config, ProxyConfig{CertificateSHA256: [][32]byte{{1}}}, capability); h != nil || err == nil {
			t.Fatal("nil/zero capability enabled proxy login")
		}
	}
}
