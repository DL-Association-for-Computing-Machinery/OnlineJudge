//go:build integration

package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPersonalEmailPlans(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	// Reuse already-existing synthetic subjects. This cardinality padding is not
	// a signup workload, password benchmark or production capacity estimate.
	_, err := pool.Exec(ctx, `INSERT INTO identity_personal_emails(subject_id,email_key,original_email)
        SELECT id,'plan-'||id||'@example.invalid','plan-'||id||'@example.invalid'
        FROM identity_credentials WHERE id BETWEEN $1 AND $2`, identityID(5000), identityID(6999))
	must(t, err)
	_, err = pool.Exec(ctx, `ANALYZE identity_personal_emails; ANALYZE identity_credentials`)
	must(t, err)
	for _, check := range []struct{ query, argument, index string }{
		{`EXPLAIN (ANALYZE,BUFFERS) SELECT c.id,c.enabled,c.credential_version,c.auth_generation,e.email_key,e.original_email,e.email_verified
            FROM identity_personal_emails e JOIN identity_credentials c ON c.id=e.subject_id WHERE e.email_key=$1`,
			"plan-" + identityID(5000) + "@example.invalid", "identity_personal_emails_email_key_key"},
		{`EXPLAIN (ANALYZE,BUFFERS) SELECT c.id,c.enabled,c.credential_version,c.auth_generation,e.email_key,e.original_email,e.email_verified
            FROM identity_personal_emails e JOIN identity_credentials c ON c.id=e.subject_id WHERE e.subject_id=$1`,
			identityID(5000), "identity_personal_emails_pkey"},
	} {
		rows, err := pool.Query(ctx, check.query, check.argument)
		must(t, err)
		var plan strings.Builder
		for rows.Next() {
			var line string
			must(t, rows.Scan(&line))
			plan.WriteString(line + "\n")
		}
		must(t, rows.Err())
		rows.Close()
		if !strings.Contains(plan.String(), check.index) || !strings.Contains(plan.String(), "identity_credentials_pkey") {
			t.Fatal("email/subject lookup lacked natural unique and credential primary-key plans")
		}
		t.Log(plan.String())
	}
	var size int64
	must(t, pool.QueryRow(ctx, `SELECT pg_indexes_size('identity_personal_emails')`).Scan(&size))
	t.Logf("Synthetic email indexes bytes: %d; not a production capacity estimate", size)
}
