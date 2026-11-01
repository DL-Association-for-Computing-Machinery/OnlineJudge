package httptransport

import (
	"net/http/httptest"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func TestLoginSocketSource(t *testing.T) {
	for raw, want := range map[string]string{"192.0.2.1:1234": "192.0.2.1", "[::ffff:192.0.2.1]:4321": "192.0.2.1", "[2001:db8::1]:1234": "2001:db8::1"} {
		source, err := loginSource(raw)
		if err != nil || source.String() != want {
			t.Fatal("socket source not canonicalized")
		}
	}
	for _, raw := range []string{"", "192.0.2.1", "proxy.invalid:1234", "192.0.2.1:0", "[fe80::1%eth0]:1234", "192.0.2.1:1,192.0.2.2:2"} {
		if _, err := loginSource(raw); err != redisstore.ErrInvalid {
			t.Fatal("invalid socket source accepted")
		}
	}
	if h, err := NewLimitedLogin(nil, Config{}); h != nil || err != redisstore.ErrInvalid {
		t.Fatal("unconfigured limited handler accepted")
	}
	w := httptest.NewRecorder()
	loginError(w, redisstore.ErrLoginRate)
	if w.Code != 429 || w.Header().Get("Retry-After") != "" || len(w.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("rate refusal revealed/reset a bucket or cookie")
	}
}
