package m0browser

import (
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestExplicitBrowserConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, flag, addr, token, session, version string
		enabled, invalid                          bool
	}{
		{name: "default closed"},
		{name: "bearer alone closed", addr: "127.0.0.1:0", token: "private-fixture", version: experiment.PinnedVersion},
		{name: "session alone closed", session: "private-fixture"},
		{"ephemeral port", "1", "127.0.0.1:0", "private-fixture", "", experiment.PinnedVersion, true, false},
		{"fixed port", "1", "127.0.0.1:12345", "private-fixture", "", experiment.PinnedVersion, true, false},
		{"unrecognized flag", "true", "127.0.0.1:0", "private-fixture", "", experiment.PinnedVersion, false, true},
		{"explicit zero flag rejected", "0", "127.0.0.1:0", "private-fixture", "", experiment.PinnedVersion, false, true},
		{"token required", "1", "127.0.0.1:0", "", "", experiment.PinnedVersion, false, true},
		{"session conflict", "1", "127.0.0.1:0", "private-fixture", "private-session", experiment.PinnedVersion, false, true},
		{"explicit address required", "1", "", "private-fixture", "", experiment.PinnedVersion, false, true},
		{"hostname rejected", "1", "localhost:12345", "private-fixture", "", experiment.PinnedVersion, false, true},
		{"wildcard rejected", "1", "0.0.0.0:12345", "private-fixture", "", experiment.PinnedVersion, false, true},
		{"other loopback rejected", "1", "127.0.0.2:12345", "private-fixture", "", experiment.PinnedVersion, false, true},
		{"IPv6 rejected", "1", "[::1]:12345", "private-fixture", "", experiment.PinnedVersion, false, true},
		{"noncanonical port", "1", "127.0.0.1:01234", "private-fixture", "", experiment.PinnedVersion, false, true},
		{"negative port", "1", "127.0.0.1:-1", "private-fixture", "", experiment.PinnedVersion, false, true},
		{"large port", "1", "127.0.0.1:65536", "private-fixture", "", experiment.PinnedVersion, false, true},
		{"v3 required", "1", "127.0.0.1:0", "private-fixture", "", "", false, true},
		{"old material rejected", "1", "127.0.0.1:0", "private-fixture", "", "m0-sum-v2", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enabled, err := Enabled(tc.flag, tc.addr, tc.token, tc.session, tc.version)
			if enabled != tc.enabled || errors.Is(err, ErrConfiguration) != tc.invalid {
				t.Fatal("unexpected explicit browser configuration result")
			}
		})
	}
}
