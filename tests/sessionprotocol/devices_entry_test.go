//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testEntryDevices(t *testing.T, f *fixture, binary string) {
	const path = "/api/v1/session/devices"
	c := runtimeConfig(t, f, namespace(t), true)
	ca := testpki.New(t)
	read := func(e *loginEndpoint, cookie []*http.Cookie) loginResponse {
		return loginCall(e, path, "", cookie, func(r *http.Request) { r.Method = http.MethodGet })
	}
	disabled := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	loginStatus(t, read(entryClient(t, disabled, ca), nil), 404)
	disabled.signal(t)
	disabled.wait(t, true)
	c.SessionDevices = true
	a := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	c.Database.ApplicationName += "_b"
	b := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	x, y := entryClient(t, a, ca), entryClient(t, b, ca)
	loginStatus(t, read(x, nil), 401)
	loginStatus(t, loginCall(x, path, "", nil, nil), 405)
	p, other := f.subject(t, false), f.subject(t, false)
	const secret = "synthetic protocol password"
	alien := entryCredential(t, y, other, secret)
	one, two, three := entryCredential(t, x, p, secret), entryCredential(t, y, p, secret), entryCredential(t, x, p, secret)
	list := func(e *loginEndpoint, credential redisstore.Credential, count int) []redisstore.Device {
		t.Helper()
		r := read(e, credentialCookies(credential))
		loginStatus(t, r, 200)
		var result struct{ Devices []redisstore.Device }
		decoder := json.NewDecoder(strings.NewReader(string(r.body)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&result) != nil || len(result.Devices) != count || len(r.cookies) != 0 ||
			r.headers.Get("Cache-Control") != "no-store" {
			t.Fatal("invalid device response")
		}
		for _, token := range []redisstore.Credential{one, two, three, alien} {
			k := tokenKey(c.Namespace, token)
			if strings.Contains(string(r.body), token.Value()) || strings.Contains(string(r.body), k[len(k)-64:]) {
				t.Fatal("device response disclosed a credential or stored digest")
			}
		}
		return result.Devices
	}
	initial := list(x, one, 3)
	if repeat := list(y, one, 3); !reflect.DeepEqual(initial, repeat) {
		t.Fatal("cross-instance list changed deadlines, identity, or order")
	}
	current := func(ds []redisstore.Device) string {
		t.Helper()
		var id string
		for _, d := range ds {
			if d.Current {
				if id != "" || len(d.ID) != 64 || d.CreatedAt.IsZero() ||
					d.AbsoluteDeadline.Sub(d.CreatedAt) != 12*time.Hour || d.IdleExpiresAt.Sub(d.CreatedAt) != 2*time.Hour {
					t.Fatal("bad current device metadata")
				}
				id = d.ID
			}
		}
		if id == "" {
			t.Fatal("device list omitted current session")
		}
		return id
	}
	id := current(initial)
	if id == current(list(x, two, 3)) || id == current(list(x, alien, 1)) {
		t.Fatal("device handles are not distinct")
	}
	key := tokenKey(c.Namespace, three)
	binding, err := f.control.HGet(f.ctx, key, "binding").Result()
	must(t, err)
	must(t, f.control.HSet(f.ctx, key, "binding", strings.Repeat("0", 64)).Err())
	loginStatus(t, read(y, credentialCookies(one)), 503)
	must(t, f.control.HSet(f.ctx, key, "binding", binding).Err())
	epoch, err := f.control.HGet(f.ctx, key, "recovery").Result()
	must(t, err)
	auth, err := f.control.HGet(f.ctx, key, "auth").Result()
	must(t, err)
	group := "oj-session:" + c.Namespace + ":subject:" + hash(p.ID+":"+auth+":"+epoch)
	idle, err := f.control.ZScore(f.ctx, group, key).Result()
	must(t, err)
	otherKey := tokenKey(c.Namespace, alien)
	otherIdle, err := f.control.HGet(f.ctx, otherKey, "idle").Int64()
	must(t, err)
	must(t, f.control.ZRem(f.ctx, group, key).Err())
	must(t, f.control.ZAdd(f.ctx, group, redisMember(otherKey, otherIdle)).Err())
	loginStatus(t, read(y, credentialCookies(one)), 503)
	must(t, f.control.ZRem(f.ctx, group, otherKey).Err())
	now, err := f.control.Time(f.ctx).Result()
	must(t, err)
	expired := now.Add(-time.Second).UnixMilli()
	must(t, f.control.ZAdd(f.ctx, group, redisMember(key, expired)).Err())
	if id != current(list(y, one, 2)) {
		t.Fatal("expired member changed the current device handle")
	}
	score, err := f.control.ZScore(f.ctx, group, key).Result()
	must(t, err)
	if score != float64(expired) {
		t.Fatal("listing modified the expired index member")
	}
	must(t, f.control.ZAdd(f.ctx, group, redisMember(key, int64(idle))).Err())
	loginStatus(t, loginCall(y, "/api/v1/session/logout", "", credentialCookies(three), nil), 204)
	if id != current(list(x, one, 2)) {
		t.Fatal("revoking a sibling changed the current device handle")
	}
	loginStatus(t, read(x, credentialCookies(three)), 401)
	for _, proc := range []*entryProcess{a, b} {
		proc.signal(t)
		proc.wait(t, true)
	}
}
