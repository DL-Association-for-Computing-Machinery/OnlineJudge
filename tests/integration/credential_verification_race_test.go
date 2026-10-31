//go:build integration

package integration_test

import (
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func testVerificationAuthorityChanges(t *testing.T, ctx context.Context, first, second, oldStore *identitypg.Store,
	h, old *password.Hasher, newVerifier func(identity.CredentialStore) *identity.CredentialVerifier) {
	n := 510
	for _, phase := range []string{"before upgrade", "current final read", "upgraded final read"} {
		for _, mutation := range []string{"replace password", "disable"} {
			t.Run(phase+"/"+mutation, func(t *testing.T) {
				n++
				writer, policy := oldStore, old
				if phase == "current final read" {
					writer, policy = first, h
				}
				row, err := writer.Initialize(ctx, identityID(n), true, identityRecord(t, policy, "synthetic original"))
				must(t, err)
				gate, entered, release := verificationGate(t)
				wrapper := &verificationBarrierStore{Store: first}
				if phase == "before upgrade" {
					wrapper.beforeRehash = gate
				} else {
					wrapper.beforeState = gate
				}
				out := startVerification(newVerifier(wrapper), ctx, row.ID, "synthetic original")
				awaitVerificationGate(t, entered)
				current, err := second.Get(ctx, row.ID)
				must(t, err)
				if phase == "upgraded final read" {
					if current.Revision != (identity.Revision{CredentialVersion: 2, AuthGeneration: 1}) {
						t.Fatal("barrier was not after committed upgrade")
					}
				} else {
					sameIdentity(t, ctx, second, row)
				}
				want := current
				if mutation == "replace password" {
					want, err = second.ReplacePasswordCAS(ctx, row.ID, current.Revision, identityRecord(t, h, "synthetic replacement"))
					must(t, err)
				} else {
					rev, err := second.DisableCAS(ctx, row.ID, current.Revision)
					must(t, err)
					want.Enabled, want.Revision = false, rev
				}
				release()
				expectVerificationFailure(t, awaitVerification(t, out), identity.ErrConflict)
				sameIdentity(t, ctx, first, want)
				if mutation == "replace password" {
					match, err := h.Verify([]byte("synthetic replacement"), want.Record)
					must(t, err)
					if !match.Match || match.NeedsUpgrade {
						t.Fatal("concurrent replacement password changed")
					}
				}
			})
		}
	}
}
