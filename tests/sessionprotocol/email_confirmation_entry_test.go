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
)

func checkPublicEmailConfirmation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, authority *identitypg.Store,
	endpoints []*loginEndpoint, owner, epoch string,
) {
	t.Helper()
	// The public mail request capability is separate. Seed a signed-up mailbox
	// and use the real issuance transaction with a disposable queue callback.
	email, err := identity.ParseEmail("confirm-after-recovery@example.invalid")
	must(t, err)
	snapshot, err := authority.Get(ctx, owner)
	must(t, err)
	const id = "0000000000000000000000000000cafe"
	account, err := authority.InitializePersonal(ctx, id, email, true, snapshot.Record)
	must(t, err)
	keys, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	reader, err := identitypg.NewEmailDeliveryReader(pool, keys)
	must(t, err)
	_, err = pool.Exec(ctx, "CREATE TABLE fixture_http_confirm_mail(id text PRIMARY KEY)")
	must(t, err)
	issue := func(p emailtoken.Purpose, gen int64) string {
		t.Helper()
		payload, err := authority.IssueEmailToken(ctx, id, identity.SessionVersion{RecoveryID: epoch, AuthGeneration: gen}, p, keys,
			func(context.Context, string) error { return nil }, func(ctx context.Context, tx pgx.Tx, id string) error {
				_, err := tx.Exec(ctx, "INSERT INTO fixture_http_confirm_mail VALUES ($1)", id)
				return err
			})
		must(t, err)
		delivery, err := reader.Prepare(ctx, payload)
		must(t, err)
		return delivery.Token().LinkValue()
	}
	post := func(endpoint *loginEndpoint, path, token, password string, want int) {
		t.Helper()
		body, err := json.Marshal(map[string]string{"token": token, "new_password": password})
		must(t, err)
		res := loginCall(endpoint, "/api/v1/account/"+path, string(body), nil, nil)
		loginStatus(t, res, want)
		if len(res.headers.Values("Set-Cookie")) != 0 {
			t.Fatal("confirmation issued or cleared a session cookie")
		}
	}
	const first = "synthetic activated mailbox password"
	const second = "synthetic recovered mailbox password"
	activation := issue(emailtoken.Activation, account.AuthGeneration)
	post(endpoints[0], "activate", activation, first, 204)
	post(endpoints[1], "activate", activation, first, 400)
	post(endpoints[0], "activate", activation, first, 429)
	fresh := entryCredential(t, endpoints[1], account, first)
	recovery := issue(emailtoken.Recovery, account.AuthGeneration+1)
	post(endpoints[1], "reset-password", recovery, second, 204)
	loginStatus(t, loginCall(endpoints[0], "/api/v1/session", "", credentialCookies(fresh), func(r *http.Request) { r.Method = http.MethodGet }), 401)
	_ = entryCredential(t, endpoints[0], account, second)
}
