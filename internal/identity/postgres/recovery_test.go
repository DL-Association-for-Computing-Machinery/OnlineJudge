package postgres

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

type failedRecoveryReader struct{}

func (failedRecoveryReader) Read([]byte) (int, error) {
	return 0, errors.New("synthetic entropy failure")
}

func TestRecoveryEntropyFailureDoesNotReachStore(t *testing.T) {
	// No database handle exists: an attempted query would fail this test. Both
	// partial entropy and a source error must stop before persistence is reached.
	s := &Store{}
	for _, source := range []io.Reader{bytes.NewReader(make([]byte, 8)), failedRecoveryReader{}} {
		generation, err := s.beginRecovery(context.Background(), source)
		if generation != "" || !errors.Is(err, identity.ErrRecoveryRandom) {
			t.Fatal("entropy failure reached persistence or returned an epoch")
		}
	}
}

func TestRecoveryMetadataRejectsMalformedClosedState(t *testing.T) {
	for _, state := range []identity.RecoveryState{
		{Generation: "invalid"}, {Generation: "0000000000000000000000000000000A"}, {Enabled: true},
	} {
		got, err := recoveryState(state.Generation, state.Enabled)
		if got != (identity.RecoveryState{}) || !errors.Is(err, identity.ErrStore) {
			t.Fatal("malformed recovery metadata was accepted")
		}
	}
	for _, state := range []identity.RecoveryState{{}, {Generation: "00000000000000000000000000000001"}, {Generation: "00000000000000000000000000000001", Enabled: true}} {
		got, err := recoveryState(state.Generation, state.Enabled)
		if got != state || err != nil {
			t.Fatal("valid closed/open metadata was rejected")
		}
	}
}
