//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testPendingDeviceReplacement(t *testing.T, f *fixture, binary string) {
	const base = "/api/v1/session/login/"
	const secret = "synthetic protocol password"
	c := runtimeConfig(t, f, namespace(t), false)
	// Keep this fixture's exchange and recovery assertions independent of the
	// production limit; every step still goes through shared quota admission.
	c.LoginLimits.RecoveryGlobal.Count = 20
	c.LoginLimits.RecoverySource.Count = 20
	secrets, err := redisstore.LoadLoginSecrets(c.SecretsFile)
	must(t, err)
	must(t, redisstore.InitializeLoginLimits(f.ctx,
		f.store(t, c.Namespace, 0, redisstore.OrdinaryPolicy()), secrets, c.LoginLimits))
	c.SessionDevices = true
	ca := testpki.New(t)
	a := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	c.Database.ApplicationName += "_b"
	b := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	x, y := entryClient(t, a, ca), entryClient(t, b, ca)
	p := f.subject(t, false)
	var original []redisstore.Credential
	for range 3 {
		original = append(original, entryCredential(t, x, p, secret))
	}
	begin := func() []*http.Cookie {
		t.Helper()
		out := loginCall(x, base+"begin", entryLoginBody(t, p, secret), nil, nil)
		loginStatus(t, out, 202)
		if len(out.cookies) != 1 {
			t.Fatal("missing original pending cookie")
		}
		return out.cookies
	}
	pending := begin()
	loginStatus(t, loginCall(x, base+"complete", "", pending, nil), 409)
	response := loginCall(y, base+"devices", "", pending, nil)
	loginStatus(t, response, 200)
	var result struct{ Devices []redisstore.Device }
	must(t, json.Unmarshal(response.body, &result))
	if len(result.Devices) != 3 || len(response.cookies) != 0 {
		t.Fatal("pending list did not contain the three active sessions")
	}
	for _, device := range result.Devices {
		if device.Current || len(device.ID) != 64 {
			t.Fatal("pending list granted current session status")
		}
	}
	selection := result.Devices[0].ID
	loginStatus(t, loginCall(x, "/api/v1/session", "", pending, func(r *http.Request) { r.Method = http.MethodGet }), 401)
	loginStatus(t, loginCall(y, base+"replace-device", selection, pending, func(r *http.Request) {
		r.Header.Set("Origin", "https://wrong.invalid")
	}), 403)
	loginStatus(t, loginCall(y, base+"replace-device", selection, begin(), nil), 409)
	loginStatus(t, loginCall(y, base+"replace-device", strings.Repeat("0", 64), pending, nil), 409)
	outcomes := make(chan loginResponse, 2)
	start := make(chan struct{})
	go func() {
		<-start
		outcomes <- loginCall(y, base+"replace-device", selection, pending, nil)
	}()
	go func() {
		<-start
		outcomes <- loginCall(x, base+"replace-device", result.Devices[1].ID, pending, nil)
	}()
	close(start)
	first, second := <-outcomes, <-outcomes
	if first.status != 204 {
		first, second = second, first
	}
	issued := completedCredential(t, first)
	loginStatus(t, second, 409)
	loginStatus(t, loginCall(x, base+"replace-device", result.Devices[1].ID, pending, nil), 409)
	recovered := completedCredential(t, loginCall(x, base+"complete", "", pending, nil))
	if recovered != issued {
		t.Fatal("completion of original attempt issued a different session")
	}
	live := 0
	for _, old := range original {
		r := loginCall(x, "/api/v1/session", "", credentialCookies(old), func(r *http.Request) { r.Method = http.MethodGet })
		if r.status == 200 {
			live++
		} else {
			loginStatus(t, r, 401)
			loginStatus(t, loginCall(y, "/api/v1/session/activity", "", credentialCookies(old), nil), 401)
		}
	}
	if live != 2 {
		t.Fatal("replacement did not revoke exactly one old session")
	}
	loginStatus(t, loginCall(x, "/api/v1/session", "", credentialCookies(issued), func(r *http.Request) { r.Method = http.MethodGet }), 200)
	for _, proc := range []*entryProcess{a, b} {
		proc.signal(t)
		proc.wait(t, true)
	}
}
