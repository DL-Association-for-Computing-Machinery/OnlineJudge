package redisstore

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

func sealingFixture(t *testing.T) (*Store, LoginConfig, *Attempt) {
	t.Helper()
	c, err := NewClient(ClientConfig{Addr: "unused.invalid:6379", Connections: 1, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	auth := &unusedAuthority{}
	gate, err := identity.NewPersonalCredentialGate(auth, &password.BoundedHasher{})
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{client: c, authority: auth, gate: gate, namespace: "login_test", policy: OrdinaryPolicy()}
	var key [32]byte
	for i := range key {
		key[i] = byte(i + 1)
	}
	cfg := LoginConfig{Origin: "https://login.example.invalid", Window: time.Minute, CurrentKey: "active", Keys: map[string][32]byte{"active": key}}
	credential, err := ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	a := &Attempt{namespace: s.namespace, credential: credential, state: identity.SessionState{State: identity.State{ID: strings.Repeat("a", 32), Enabled: true,
		Revision: identity.Revision{CredentialVersion: 7, AuthGeneration: 8}}, RecoveryID: strings.Repeat("b", 32)},
		created: 1000, initial: 1000 + s.policy.Idle.Milliseconds(), absolute: 1000 + s.policy.Absolute.Milliseconds(), loginUntil: 61000}
	return s, cfg, a
}

func TestSealedLoginRoundTripAndRotation(t *testing.T) {
	s, cfg, a := sealingFixture(t)
	l, err := NewLogin(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	one, err := l.seal(a)
	if err != nil {
		t.Fatal(err)
	}
	two, err := l.seal(a)
	if err != nil {
		t.Fatal(err)
	}
	if one.Value() == two.Value() || one.Deadline() != time.UnixMilli(a.loginUntil) {
		t.Fatal("nonce or frozen deadline not preserved")
	}
	parsed, err := ParseContinuation(one.Value())
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Deadline().IsZero() {
		t.Fatal("parsing browser input trusted its deadline")
	}
	opened, err := l.open(parsed)
	if err != nil || !reflect.DeepEqual(a, opened) {
		t.Fatal("frozen tuple was not restored")
	}
	var newer [32]byte
	for i := range newer {
		newer[i] = byte(i + 40)
	}
	cfg.Keys["next"] = newer
	cfg.CurrentKey = "next"
	restarted, err := NewLogin(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.open(parsed); err != nil {
		t.Fatal("old-key continuation rejected during rotation")
	}
	fresh, err := restarted.seal(a)
	if err != nil || !strings.HasPrefix(fresh.Value(), "1.next.") {
		t.Fatal("rotation did not use current key")
	}
	delete(cfg.Keys, "active")
	cfg.Keys["next"] = [32]byte{}
	if _, err = restarted.open(parsed); err != nil {
		t.Fatal("caller map mutation changed manager")
	}
	if _, err = restarted.open(fresh); err != nil {
		t.Fatal("caller key mutation changed AEAD")
	}
	for _, v := range []any{one, &one, parsed, cfg, l} {
		text := fmt.Sprintf("%v %+v %#v", v, v, v)
		if strings.Contains(text, one.Value()) || strings.Contains(text, a.credential.Value()) {
			t.Fatal("sensitive login value leaked via formatting")
		}
	}
}

func TestSealedLoginIsolationAndTampering(t *testing.T) {
	s, cfg, a := sealingFixture(t)
	l, err := NewLogin(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	c, err := l.seal(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"origin", "namespace", "idle", "absolute", "window", "key", "keyID"} {
		t.Run(change, func(t *testing.T) {
			candidate := c
			otherStore := *s
			otherCfg := cfg
			otherCfg.Keys = make(map[string][32]byte)
			for k, v := range cfg.Keys {
				otherCfg.Keys[k] = v
			}
			switch change {
			case "origin":
				otherCfg.Origin = "https://other.example.invalid"
			case "namespace":
				otherStore.namespace = "other"
			case "idle":
				otherStore.policy.Idle = time.Hour
			case "absolute":
				otherStore.policy.Absolute = 10 * time.Hour
			case "window":
				otherCfg.Window = 30 * time.Second
			case "key":
				k := otherCfg.Keys["active"]
				k[0] ^= 1
				otherCfg.Keys["active"] = k
			case "keyID":
				otherCfg.Keys["alias"] = otherCfg.Keys["active"]
				otherCfg.CurrentKey = "alias"
				candidate.value = strings.Replace(c.value, "1.active.", "1.alias.", 1)
			}
			other, err := NewLogin(&otherStore, otherCfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = other.open(candidate); err != ErrInvalid {
				t.Fatal("changed sealing context accepted")
			}
		})
	}
	for _, bad := range []string{"", c.Value() + "=", strings.Replace(c.Value(), "1.", "2.", 1), strings.Repeat("x", 1537), "1.active.AAAA"} {
		if _, err := ParseContinuation(bad); err != ErrInvalid {
			t.Fatal("invalid continuation syntax accepted")
		}
	}
	parts := strings.Split(c.Value(), ".")
	raw, _ := base64.RawURLEncoding.DecodeString(parts[2])
	raw[len(raw)-1] ^= 1
	bad, err := ParseContinuation(parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Complete(context.Background(), bad); err != ErrInvalid {
		t.Fatal("tampered envelope reached session protocol")
	}
	if err := l.Abandon(context.Background(), bad); err != ErrInvalid {
		t.Fatal("tampered envelope reached abandonment")
	}
}

func TestLoginConfigurationAndFrozenWindow(t *testing.T) {
	s, cfg, a := sealingFixture(t)
	for _, change := range []string{"origin", "path", "short", "long", "fraction", "missing", "zero key", "bad ID", "many"} {
		t.Run(change, func(t *testing.T) {
			c := cfg
			c.Keys = map[string][32]byte{"active": cfg.Keys["active"]}
			switch change {
			case "origin":
				c.Origin = "http://login.example.invalid"
			case "path":
				c.Origin += "/"
			case "short":
				c.Window = 0
			case "long":
				c.Window = 6 * time.Minute
			case "fraction":
				c.Window = time.Nanosecond
			case "missing":
				c.CurrentKey = "absent"
			case "zero key":
				c.Keys["active"] = [32]byte{}
			case "bad ID":
				c.CurrentKey = "bad.key"
			case "many":
				for _, id := range []string{"a", "b", "c", "d"} {
					c.Keys[id] = c.Keys["active"]
				}
			}
			if v, err := NewLogin(s, c); err != ErrInvalid || v != nil {
				t.Fatal("invalid login configuration accepted")
			}
		})
	}
	l, err := NewLogin(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, until := range []int64{0, a.created, a.initial + 1, a.loginUntil + 1} {
		bad := *a
		bad.loginUntil = until
		if _, err := l.seal(&bad); err != ErrInvalid {
			t.Fatal("candidate window was changed or extended")
		}
	}
	for _, bad := range []*Login{nil, {}} {
		if c, err := bad.Prepare(context.Background(), identity.Email{}, nil); err != ErrInvalid || c != (Continuation{}) {
			t.Fatal("zero manager prepared login")
		}
		if c, err := bad.Complete(context.Background(), Continuation{}); err != ErrInvalid || c != (Credential{}) {
			t.Fatal("zero manager completed login")
		}
		if err := bad.Abandon(context.Background(), Continuation{}); err != ErrInvalid {
			t.Fatal("zero manager abandoned login")
		}
	}
}
