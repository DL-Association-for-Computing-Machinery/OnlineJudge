package main

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/m0browser"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/apiapp"
)

func TestSessionModeRejectsAmbiguousStartupBeforeLoading(t *testing.T) {
	for _, tc := range []struct{ name, address, experiment string }{
		{"missing explicit address", "", ""},
		{"experiment conflict", "127.0.0.1:0", "/never-read-synthetic-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OJ_SESSION_CONFIG_FILE", "/never-read-synthetic-session-config")
			t.Setenv("OJ_API_ADDR", tc.address)
			t.Setenv("OJ_EXPERIMENT_TOKEN_FILE", tc.experiment)
			if err := run(slog.New(slog.NewJSONHandler(io.Discard, nil))); !errors.Is(err, apiapp.ErrConfiguration) {
				t.Fatal("ambiguous session mode reached file or listener setup")
			}
		})
	}
}

func TestBrowserStartupRejectsBeforeReadingPrivateConfiguration(t *testing.T) {
	for _, tc := range []struct{ name, flag, address, token, session, version string }{
		{"missing token", "1", "127.0.0.1:0", "", "", "m0-sum-v3"},
		{"wrong flag", "true", "127.0.0.1:0", "/never-read-token", "", "m0-sum-v3"},
		{"session conflict", "1", "127.0.0.1:0", "/never-read-token", "/never-read-session", "m0-sum-v3"},
		{"non-loopback", "1", "0.0.0.0:12345", "/never-read-token", "", "m0-sum-v3"},
		{"wrong version", "1", "127.0.0.1:0", "/never-read-token", "", "m0-sum-v2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OJ_EXPERIMENT_BROWSER", tc.flag)
			t.Setenv("OJ_API_ADDR", tc.address)
			t.Setenv("OJ_EXPERIMENT_TOKEN_FILE", tc.token)
			t.Setenv("OJ_SESSION_CONFIG_FILE", tc.session)
			t.Setenv("OJ_EXPERIMENT_MATERIAL_VERSION", tc.version)
			if err := run(slog.New(slog.NewJSONHandler(io.Discard, nil))); !errors.Is(err, m0browser.ErrConfiguration) {
				t.Fatal("browser conflict reached private file or listener setup")
			}
		})
	}
}
