package apiconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

const validDocument = `{
 "version":1,"mode":"direct","namespace":"api-test","secrets_file":"/config/login.json",
 "origin":"https://oj.invalid","login_window":"1m","startup_timeout":"3s","http_timeout":"1s",
 "database":{"host":"db.invalid","port":5432,"database":"fixture","username":"fixture",
  "application_name":"api-test","security":"private","auth":"env","connections":2,"timeout":"1s",
  "max_lifetime":"1h","max_idle":"1m","health_check":"1s","tls":{}},
 "redis":{"address":"redis.invalid:6379","username":"","security":"private","auth":"none",
  "connections":2,"timeout":"1s","tls":{}},
 "password":{"current":{"memory_kib":128,"iterations":2,"parallelism":2,"salt_bytes":16,"digest_bytes":32},
  "legacy":[],"limits":{"max_memory_kib":256,"max_iterations":4,"max_parallelism":4,
   "max_password_bytes":128,"max_salt_bytes":64,"max_digest_bytes":64},
  "admission":{"max_in_flight":2,"max_waiting":0,"max_queue_wait":"0s"}},
 "login_limits":{"begin_global":{"count":10,"window":"1h"},"begin_source":{"count":10,"window":"1h"},
  "begin_email":{"count":10,"window":"1h"},"recovery_global":{"count":10,"window":"1h"},
  "recovery_source":{"count":10,"window":"1h"}},
 "proxy_fingerprints":[],"tls":{"certificate_file":"/config/server.pem","key_file":"/config/server.key"},
 "server":{"read_header_timeout":"2s","read_timeout":"5s","write_timeout":"5s","idle_timeout":"1m",
  "shutdown_timeout":"3s","max_header_bytes":8192}
}`

func TestFullConfigurationPreservesNativeValues(t *testing.T) {
	c, err := Parse([]byte(validDocument))
	if err != nil {
		t.Fatal("valid configuration rejected")
	}
	r := c.Runtime
	if r.ManagementSessions {
		t.Fatal("omitted management configuration enabled privileged routes")
	}
	if r.Mode != sessionruntime.DirectTLS || r.Database.Security != sessionruntime.PrivateTransport ||
		r.RedisSecurity != sessionruntime.PrivateTransport || r.Namespace != "api-test" ||
		r.SecretsFile != "/config/login.json" || r.Database.Host != "db.invalid" || r.Database.Port != 5432 ||
		r.Database.Database != "fixture" || r.Database.Username != "fixture" || r.Database.ApplicationName != "api-test" ||
		r.Database.Connections != 2 || r.Database.Timeout != time.Second || r.Database.MaxLifetime != time.Hour ||
		r.Database.MaxIdle != time.Minute || r.Database.HealthCheck != time.Second ||
		r.Redis.Addr != "redis.invalid:6379" || r.Redis.Connections != 2 || r.Redis.Timeout != time.Second ||
		r.Database.Password != "" || r.Redis.Password != "" || r.Database.TLS != nil || r.Redis.TLS != nil {
		t.Fatal("native endpoint/security values changed")
	}
	if r.HTTP.Origin != "https://oj.invalid" || r.HTTP.Timeout != time.Second || r.LoginWindow != time.Minute ||
		r.StartupTimeout != 3*time.Second || r.PasswordPolicy.Current != (password.Parameters{
		MemoryKiB: 128, Iterations: 2, Parallelism: 2, SaltBytes: 16, DigestBytes: 32}) ||
		r.PasswordLimits.MaxPasswordBytes != 128 || r.PasswordAdmission.MaxInFlight != 2 ||
		r.PasswordAdmission.MaxWaiting != 0 || r.PasswordAdmission.MaxQueueWait != 0 ||
		r.LoginLimits.RecoverySource.Count != 10 || r.LoginLimits.BeginEmail.Window != time.Hour {
		t.Fatal("native policy/budgets changed")
	}
	if c.DatabaseAuth != "env" || c.RedisAuth != "none" || c.TLS.CertificateFile != "/config/server.pem" ||
		c.DatabaseTLS != (Material{}) || c.RedisTLS != (Material{}) ||
		c.Server.ReadHeaderTimeout != 2*time.Second || c.Server.ReadTimeout != 5*time.Second ||
		c.Server.WriteTimeout != 5*time.Second || c.Server.IdleTimeout != time.Minute ||
		c.Server.ShutdownTimeout != 3*time.Second || c.Server.MaxHeaderBytes != 8192 {
		t.Fatal("API transport settings changed")
	}
}

func TestManagementSessionsOptIn(t *testing.T) {
	with := func(value string) []byte {
		return []byte(strings.Replace(validDocument, `"version":1,`, `"version":1,"management_sessions":`+value+`,`, 1))
	}
	c, err := Parse(with("true"))
	if err != nil || !c.Runtime.ManagementSessions {
		t.Fatal("explicit management configuration was not preserved")
	}
	reject(t, with(`"true"`))
	reject(t, with(`null`))
	roleChanges := strings.Replace(validDocument, `"version":1,`, `"version":1,"management_role_changes":true,`, 1)
	reject(t, []byte(roleChanges))
	c, err = Parse([]byte(strings.Replace(roleChanges, `"version":1,`, `"version":1,"management_sessions":true,`, 1)))
	if err != nil || !c.Runtime.ManagementRoleChanges {
		t.Fatal("explicit role writes were not preserved")
	}
}

func reject(t *testing.T, body []byte) {
	t.Helper()
	got, err := Parse(body)
	if !errors.Is(err, ErrConfig) || err != ErrConfig || err.Error() != "invalid session API configuration" ||
		got.Runtime.Mode != 0 || got.TLS != (Material{}) {
		t.Fatal("invalid configuration was not rejected with sanitized zero result")
	}
}

func TestMalformedAndAmbiguousJSONRejected(t *testing.T) {
	for name, body := range map[string]string{
		"empty": "", "root null": "null", "root array": "[]", "root string": `"fixture"`,
		"syntax": `{`, "trailing object": validDocument + `{}`, "trailing token": validDocument + ` true`,
		"duplicate":          strings.Replace(validDocument, `"version":1`, `"version":1,"version":1`, 1),
		"escaped duplicate":  strings.Replace(validDocument, `"version":1`, `"version":1,"v\u0065rsion":1`, 1),
		"nested duplicate":   strings.Replace(validDocument, `"port":5432`, `"port":5432,"port":5432`, 1),
		"case alias":         strings.Replace(validDocument, `"version"`, `"Version"`, 1),
		"unknown":            strings.Replace(validDocument, `"version":1`, `"unrecognized":1,"version":1`, 1),
		"password input":     strings.Replace(validDocument, `"auth":"env"`, `"auth":"env","password":"synthetic-fixture"`, 1),
		"surrogate":          strings.Replace(validDocument, `"api-test"`, `"\ud800"`, 1),
		"nested null":        strings.Replace(validDocument, `"database":{`, `"database":null,"unused":{`, 1),
		"array null":         strings.Replace(validDocument, `"legacy":[]`, `"legacy":[null]`, 1),
		"scalar null":        strings.Replace(validDocument, `"max_waiting":0`, `"max_waiting":null`, 1),
		"missing queue":      strings.Replace(validDocument, `"max_waiting":0,`, ``, 1),
		"missing material":   strings.Replace(validDocument, `,"tls":{}`, ``, 1),
		"missing legacy":     strings.Replace(validDocument, `"legacy":[],`, ``, 1),
		"missing proxy list": strings.Replace(validDocument, `"proxy_fingerprints":[],`, ``, 1),
		"missing login":      strings.Replace(validDocument, `"login_window":"1m",`, ``, 1),
	} {
		t.Run(name, func(t *testing.T) { reject(t, []byte(body)) })
	}
	reject(t, append([]byte(validDocument[:20]), 0xff))
	if _, err := Parse([]byte(strings.Replace(validDocument, `"version"`, `"v\u0065rsion"`, 1))); err != nil {
		t.Fatal("canonical decoded key rejected")
	}
}

func TestTypedBudgetsAndPoliciesRejected(t *testing.T) {
	for name, pair := range map[string][2]string{
		"version":               {`"version":1`, `"version":2`},
		"version float":         {`"version":1`, `"version":1.0`},
		"port overflow":         {`"port":5432`, `"port":65536`},
		"negative uint":         {`"port":5432`, `"port":-1`},
		"lane overflow":         {`"parallelism":2`, `"parallelism":256`},
		"memory overflow":       {`"memory_kib":128`, `"memory_kib":4294967296`},
		"signed overflow":       {`"max_waiting":0`, `"max_waiting":9223372036854775808`},
		"fraction":              {`"connections":2`, `"connections":2.5`},
		"duration number":       {`"http_timeout":"1s"`, `"http_timeout":1`},
		"duration missing unit": {`"http_timeout":"1s"`, `"http_timeout":"1"`},
		"duration overflow":     {`"http_timeout":"1s"`, `"http_timeout":"999999999999999999999h"`},
		"empty duration":        {`"http_timeout":"1s"`, `"http_timeout":""`},
		"HTTP cap":              {`"http_timeout":"1s"`, `"http_timeout":"6s"`},
		"startup cap":           {`"startup_timeout":"3s"`, `"startup_timeout":"61s"`},
		"bad origin":            {`https://oj.invalid`, `http://oj.invalid`},
		"namespace":             {`"namespace":"api-test"`, `"namespace":"INVALID"`},
		"login rate":            {`"count":10`, `"count":0`},
		"login window":          {`"login_window":"1m"`, `"login_window":"0s"`},
		"password tuple":        {`"memory_kib":128`, `"memory_kib":1`},
		"record size":           {`"salt_bytes":16`, `"salt_bytes":65`},
		"admission cap":         {`"max_in_flight":2`, `"max_in_flight":1025`},
		"queue cap":             {`"max_waiting":0`, `"max_waiting":65537`},
		"queue wait":            {`"max_waiting":0`, `"max_waiting":1`},
		"header cap":            {`"max_header_bytes":8192`, `"max_header_bytes":1048577`},
		"header minimum":        {`"max_header_bytes":8192`, `"max_header_bytes":1023`},
		"read order":            {`"read_timeout":"5s"`, `"read_timeout":"1s"`},
		"write order":           {`"write_timeout":"5s"`, `"write_timeout":"500ms"`},
		"shutdown cap":          {`"shutdown_timeout":"3s"`, `"shutdown_timeout":"61s"`},
		"idle cap":              {`"idle_timeout":"1m"`, `"idle_timeout":"11m"`},
		"DB pool missing":       {`"connections":2`, `"connections":0`},
		"DB duration missing":   {`"max_idle":"1m",`, ``},
	} {
		t.Run(name, func(t *testing.T) { reject(t, []byte(strings.Replace(validDocument, pair[0], pair[1], 1))) })
	}
}

func TestModesAuthenticationAndMaterial(t *testing.T) {
	for name, pair := range map[string][2]string{
		"unknown mode":             {`"mode":"direct"`, `"mode":"fallback"`},
		"unknown auth":             {`"auth":"env"`, `"auth":"password"`},
		"arbitrary env":            {`"auth":"env"`, `"auth":"CUSTOM_ENV"`},
		"unknown security":         {`"security":"private"`, `"security":"insecure"`},
		"private TLS":              {`"tls":{}`, `"tls":{"ca_file":"/config/ca.pem"}`},
		"TLS missing identity":     {`"security":"private"`, `"security":"tls"`},
		"direct pins":              {`"proxy_fingerprints":[]`, `"proxy_fingerprints":["` + strings.Repeat("1", 64) + `"]`},
		"proxy missing pins":       {`"mode":"direct"`, `"mode":"proxy"`},
		"listener CA unused":       {`"key_file":"/config/server.key"`, `"key_file":"/config/server.key","ca_file":"/config/ca.pem"`},
		"listener identity unused": {`"key_file":"/config/server.key"`, `"key_file":"/config/server.key","server_name":"oj.invalid"`},
		"relative path":            {`/config/server.pem`, `server.pem`},
		"unclean path":             {`/config/server.pem`, `/config/../server.pem`},
		"path NUL":                 {`/config/server.pem`, `/config/server\u0000.pem`},
		"path cap":                 {`/config/server.pem`, `/` + strings.Repeat("a", 4096)},
		"missing cert":             {`"certificate_file":"/config/server.pem",`, ``},
		"relative secrets":         {`/config/login.json`, `login.json`},
	} {
		t.Run(name, func(t *testing.T) { reject(t, []byte(strings.Replace(validDocument, pair[0], pair[1], 1))) })
	}

	var doc map[string]any
	if json.Unmarshal([]byte(validDocument), &doc) != nil {
		t.Fatal("fixture decode")
	}
	doc["mode"] = "proxy"
	doc["proxy_fingerprints"] = []string{strings.Repeat("ab", 32)}
	doc["tls"].(map[string]any)["ca_file"] = "/config/proxy-ca.pem"
	for _, name := range []string{"database", "redis"} {
		store := doc[name].(map[string]any)
		store["security"] = "tls"
		store["tls"] = map[string]any{"ca_file": "/config/store-ca.pem", "server_name": name + ".invalid"}
	}
	body, _ := json.Marshal(doc)
	c, err := Parse(body)
	if err != nil || c.Runtime.Mode != sessionruntime.ProxyMTLS || c.Runtime.Proxy.CertificateSHA256[0][0] != 0xab ||
		c.Runtime.Database.Security != sessionruntime.VerifiedTLS || c.Runtime.RedisSecurity != sessionruntime.VerifiedTLS ||
		c.Runtime.Database.TLS != nil || c.Runtime.Redis.TLS != nil {
		t.Fatal("proxy or deferred storage TLS configuration rejected")
	}
	for name, pins := range map[string][]string{
		"zero": {strings.Repeat("0", 64)}, "uppercase": {strings.Repeat("AB", 32)},
		"duplicate": {strings.Repeat("ab", 32), strings.Repeat("ab", 32)}, "bad hex": {strings.Repeat("z", 64)},
	} {
		t.Run(name, func(t *testing.T) {
			doc["proxy_fingerprints"] = pins
			body, _ := json.Marshal(doc)
			reject(t, body)
		})
	}
}

func TestStructuralWorkBounds(t *testing.T) {
	reject(t, []byte(validDocument+strings.Repeat(" ", maxBytes)))
	if _, err := Parse([]byte(validDocument + strings.Repeat(" ", maxBytes-len(validDocument)))); err != nil {
		t.Fatal("exact input byte boundary rejected")
	}
	accepted := func(body string) bool {
		decoder := json.NewDecoder(strings.NewReader(body))
		decoder.UseNumber()
		keys := 0
		return scanValue(decoder, 0, &keys) == nil
	}
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"depth boundary", `{"extra":` + strings.Repeat(`[`, 11) + `0` + strings.Repeat(`]`, 11) + `}`, true},
		{"depth overflow", `{"extra":` + strings.Repeat(`[`, 12) + `0` + strings.Repeat(`]`, 12) + `}`, false},
		{"array boundary", `{"extra":[` + strings.Repeat(`0,`, 15) + `0]}`, true},
		{"array overflow", `{"extra":[` + strings.Repeat(`0,`, 16) + `0]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if accepted(tc.body) != tc.want {
				t.Fatal("unexpected container work-bound result")
			}
		})
	}
	var members []string
	for index := 0; index < 257; index++ {
		key, _ := json.Marshal("key" + strings.Repeat("a", index))
		members = append(members, string(key)+`:0`)
	}
	if !accepted(`{`+strings.Join(members[:256], `,`)+`}`) || accepted(`{`+strings.Join(members, `,`)+`}`) {
		t.Fatal("unexpected total key work-bound result")
	}
	// Invalid UTF-8 is embedded in otherwise valid JSON, which the standard
	// decoder alone would replace instead of rejecting.
	reject(t, bytes.Replace([]byte(validDocument), []byte("api-test"), []byte{0xff}, 1))
}
