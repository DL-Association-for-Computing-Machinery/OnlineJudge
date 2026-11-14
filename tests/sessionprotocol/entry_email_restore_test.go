//go:build integration

package sessionprotocol_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
)

type archivedRecoveryLink struct {
	account identity.Personal
	keys    *emailtoken.Protector
	payload string
	token   emailtoken.Token
}

const reconciledMailboxPassword = "synthetic post-backup mailbox password"

func prepareArchivedRecoveryLink(t *testing.T, f *fixture) archivedRecoveryLink {
	t.Helper()
	// Use a separate mailbox to preserve the archive's ordinary login fixture.
	mailbox := f.subject(t, false)
	_, err := f.pools[0].Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1`, mailbox.ID)
	must(t, err)
	state, err := f.identities[0].RecoveryState(f.ctx)
	must(t, err)
	keys, err := emailtoken.NewProtector("archive-fixture", map[string][32]byte{"archive-fixture": {1}})
	must(t, err)
	_, err = f.pools[0].Exec(f.ctx, `CREATE TABLE fixture_archived_mail (id text PRIMARY KEY)`)
	must(t, err)
	payload, err := f.identities[0].IssueEmailToken(f.ctx, mailbox.ID,
		identity.SessionVersion{RecoveryID: state.Generation, AuthGeneration: mailbox.AuthGeneration}, emailtoken.Recovery, keys,
		func(context.Context, *emailtoken.Protector) error { return nil }, func(ctx context.Context, tx pgx.Tx, id string) error {
			_, err := tx.Exec(ctx, `INSERT INTO fixture_archived_mail VALUES ($1)`, id)
			return err
		})
	must(t, err)
	reader, err := identitypg.NewEmailDeliveryReader(f.pools[0], keys)
	must(t, err)
	delivery, err := reader.Prepare(f.ctx, payload)
	must(t, err)
	return archivedRecoveryLink{mailbox, keys, payload, delivery.Token()}
}

func (m archivedRecoveryLink) consumeAtSource(t *testing.T, f *fixture) {
	t.Helper()
	record, err := f.hasher.Create([]byte(reconciledMailboxPassword))
	must(t, err)
	must(t, f.identities[0].ResetPasswordByEmail(f.ctx, m.token, record))
	wantError(t, f.identities[0].ResetPasswordByEmail(f.ctx, m.token, record), identity.ErrConflict)
}

func (m archivedRecoveryLink) reconcile(t *testing.T, ctx context.Context, f *fixture, target *identitypg.Store) {
	t.Helper()
	// The test's isolated source represents independently confirmed newer
	// account facts. Never treat the restored password as current by default.
	newer, err := f.identities[0].Get(ctx, m.account.ID)
	must(t, err)
	updated, err := target.ReplacePasswordCAS(ctx, m.account.ID, m.account.Revision, newer.Record)
	must(t, err)
	if updated.Revision != newer.Revision {
		t.Fatal("restored credentials did not match the post-backup revision")
	}
}

func (m archivedRecoveryLink) checkArchivedAuthority(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	reader, err := identitypg.NewEmailDeliveryReader(pool, m.keys)
	must(t, err)
	delivery, err := reader.Prepare(ctx, m.payload)
	must(t, err)
	if delivery.Token() != m.token {
		t.Fatal("archived mail was not eligible before rotation")
	}
}

func (m archivedRecoveryLink) checkReopened(t *testing.T, ctx context.Context, pool *pgxpool.Pool, endpoints ...*loginEndpoint) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"token": m.token.LinkValue(), "new_password": "synthetic rejected archived password"})
	must(t, err)
	for _, e := range endpoints {
		r := loginCall(e, "/api/v1/account/reset-password", string(body), nil, nil)
		loginStatus(t, r, 400)
	}
	m.checkDeliveryFenced(t, ctx, pool)
	var unchanged bool
	must(t, pool.QueryRow(ctx, `SELECT auth_generation=$2 AND credential_version=$3 FROM identity_credentials WHERE id=$1`,
		m.account.ID, m.account.AuthGeneration+1, m.account.CredentialVersion+1).Scan(&unchanged))
	if !unchanged {
		t.Fatal("archived email confirmation changed the account")
	}
	loginStatus(t, loginCall(endpoints[0], "/api/v1/session/login/begin", loginBodyFor(t, m.account), nil, nil), 401)
	current := entryCredential(t, endpoints[0], m.account, reconciledMailboxPassword)
	assertManagementRead(t, endpoints[1], "/api/v1/session", &http.Cookie{Name: transport.CookieName, Value: current.Value()}, 200)
	t.Log("archived recovery link was still deliverable before fencing, then rejected by both API processes and by the delivery reader")
}

func (m archivedRecoveryLink) checkDeliveryFenced(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	reader, err := identitypg.NewEmailDeliveryReader(pool, m.keys)
	must(t, err)
	_, err = reader.Prepare(ctx, m.payload)
	wantError(t, err, identity.ErrConflict)
}
