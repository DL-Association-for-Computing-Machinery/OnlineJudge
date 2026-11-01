//go:build integration

package sessionprotocol_test

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func testLogin(t *testing.T, f *fixture) {
	f.open(t)
	t.Run("two TLS replicas and recreated manager with shared cookie jar", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		key := loginKey(t)
		limiting := loginKey(t)
		a := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		b := f.store(t, ns, 1, protocol.OrdinaryPolicy())
		x := loginServer(t, loadedLoginManager(t, a, time.Minute, key, limiting))
		y := loginServer(t, loadedLoginManager(t, b, time.Minute, key, limiting))
		jar, err := cookiejar.New(nil)
		must(t, err)
		x.client.Jar = jar
		y.client.Jar = jar
		pending := beginLogin(t, x, p)
		if len(namespaceRecords(t, f, ns)) != 0 {
			t.Fatal("prepare occupied a session slot")
		}
		loginStatus(t, loginCall(y, "/api/v1/session", "", nil, func(r *http.Request) { r.Method = "GET" }), 401)
		credential := completedCredential(t, loginCall(y, "/api/v1/session/login/complete", "", nil, nil))
		u, err := url.Parse(loginOrigin)
		must(t, err)
		cookies := jar.Cookies(u)
		if len(cookies) != 1 || cookies[0].Name != transport.CookieName || cookies[0].Value != credential.Value() {
			t.Fatal("cookie jar did not replace pending with session")
		}
		loginStatus(t, loginCall(x, "/api/v1/session", "", nil, func(r *http.Request) { r.Method = "GET" }), 200)
		loginStatus(t, loginCall(x, "/api/v1/session/login/begin", loginBodyFor(t, p), nil, nil), 409)
		loginStatus(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 409)
		values, err := f.control.HGetAll(f.ctx, tokenKey(ns, credential)).Result()
		must(t, err)
		for _, v := range values {
			if v == credential.Value() || strings.Contains(v, "synthetic protocol password") {
				t.Fatal("Redis retained plaintext credentials")
			}
		}
		loginStatus(t, loginCall(x, "/api/v1/session/logout", "", nil, nil), 204)
		if len(jar.Cookies(u)) != 0 {
			t.Fatal("confirmed logout did not clear jar")
		}
	})
	t.Run("lost prepare response and parallel begin have no session effects", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		key := loginKey(t)
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		x := loginServer(t, loginManager(t, s, time.Minute, key))
		out := make(chan loginResponse, 2)
		body := loginBodyFor(t, p)
		for range 2 {
			go func() { out <- loginCall(x, "/api/v1/session/login/begin", body, nil, nil) }()
		}
		one, two := <-out, <-out
		loginStatus(t, one, 202)
		loginStatus(t, two, 202)
		lost, other := responseCookie(t, one, transport.AttemptCookieName), responseCookie(t, two, transport.AttemptCookieName)
		if lost.Value == other.Value || len(namespaceRecords(t, f, ns)) != 0 {
			t.Fatal("unreceived preparations occupied slots or reused entropy")
		}
		loginStatus(t, loginCall(x, "/api/v1/session/login/begin", loginBodyFor(t, p), []*http.Cookie{other}, nil), 409)
		completedCredential(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{other}, nil))
		if len(namespaceRecords(t, f, ns)) != 1 {
			t.Fatal("lost prepare response left an admission")
		}
	})
	t.Run("concurrent complete reuses one token and original receipt", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		key := loginKey(t)
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		other := f.store(t, ns, 1, protocol.OrdinaryPolicy())
		ends := [2]*loginEndpoint{loginServer(t, loginManager(t, s, time.Minute, key)), loginServer(t, loginManager(t, other, time.Minute, key))}
		pending := beginLogin(t, ends[0], p)
		out := make(chan loginResponse, 12)
		start := make(chan struct{})
		for i := range 12 {
			go func() {
				<-start
				out <- loginCall(ends[i%2], "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil)
			}()
		}
		close(start)
		var credential protocol.Credential
		for range 12 {
			c := completedCredential(t, <-out)
			if credential == (protocol.Credential{}) {
				credential = c
			} else if credential != c {
				t.Fatal("same attempt admitted multiple tokens")
			}
		}
		if len(namespaceRecords(t, f, ns)) != 1 {
			t.Fatal("concurrent completion created extra admissions")
		}
		before, err := f.control.HGetAll(f.ctx, tokenKey(ns, credential)).Result()
		must(t, err)
		completedCredential(t, loginCall(ends[1], "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
		after, err := f.control.HGetAll(f.ctx, tokenKey(ns, credential)).Result()
		must(t, err)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("retry changed original metadata/deadlines")
		}
		v, err := s.Read(f.ctx, credential)
		must(t, err)
		f.waitTime(t, v.CreatedAt.Add(20*time.Millisecond))
		_, err = other.Touch(f.ctx, credential)
		must(t, err)
		loginStatus(t, loginCall(ends[0], "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 409)
		if len(namespaceRecords(t, f, ns)) != 1 {
			t.Fatal("conflict triggered new admission")
		}
		abandoned := loginCall(ends[1], "/api/v1/session/login/abandon", "", []*http.Cookie{pending}, nil)
		loginStatus(t, abandoned, 204)
		if len(abandoned.cookies) != 1 || responseCookie(t, abandoned, transport.AttemptCookieName).MaxAge != -1 {
			t.Fatal("abandon changed session cookie")
		}
		loginStatus(t, loginCall(ends[0], "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 401)
		_, err = s.Read(f.ctx, credential)
		wantError(t, err, protocol.ErrStale)
	})
	t.Run("expired old session permits fresh login without premature clearing", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		s := f.store(t, ns, 0, protocol.Policy{Idle: time.Second, Absolute: 3 * time.Second})
		x := loginServer(t, loginManager(t, s, 500*time.Millisecond, loginKey(t)))
		_, old := f.create(t, s, p)
		v, err := s.Read(f.ctx, old)
		must(t, err)
		f.waitTime(t, v.IdleExpiresAt)
		oldCookie := &http.Cookie{Name: transport.CookieName, Value: old.Value()}
		r := loginCall(x, "/api/v1/session/login/begin", loginBodyFor(t, p), []*http.Cookie{oldCookie}, nil)
		loginStatus(t, r, 202)
		if len(r.cookies) != 1 || r.cookies[0].Name != transport.AttemptCookieName {
			t.Fatal("prepare cleared/replaced old browser credential")
		}
		pending := responseCookie(t, r, transport.AttemptCookieName)
		fresh := completedCredential(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending, oldCookie}, nil))
		if fresh == old {
			t.Fatal("reauthentication reused expired token")
		}
		_, err = s.Read(f.ctx, fresh)
		must(t, err)
	})
	t.Run("malformed and cross origin requests fail before Redis", func(t *testing.T) { testLoginPreflight(t, f) })
	t.Run("unknown receipts recover only the same attempt", func(t *testing.T) { testLoginUnknown(t, f) })
	t.Run("actual execution deadline and abandonment", func(t *testing.T) { testLoginDeadline(t, f) })
	t.Run("credential and recovery fences", func(t *testing.T) { testLoginFences(t, f) })
}
