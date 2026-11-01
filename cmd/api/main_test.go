package main

import (
	"errors"
	"io"
	"log/slog"
	"testing"

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
