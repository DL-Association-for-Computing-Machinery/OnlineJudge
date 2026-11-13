//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

const entryTrainingHistory = "/api/v1/training/submissions"

// Both real HTTPS fixtures use these rows. The material and verdicts are
// synthetic; no submission API, publication, or judge execution is involved.
func seedTrainingHistory(t *testing.T, f *fixture, subject, other string) {
	t.Helper()
	const version = "synthetic-history-material"
	tx, err := f.pools[0].Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `INSERT INTO submission_materials (version_id, manifest_sha256, manifest)
		VALUES ($1, repeat('a', 64), '{}')`, version)
	must(t, err)
	_, err = tx.Exec(f.ctx, `WITH accepted AS (
		INSERT INTO submissions (id, subject_id, context_id, idempotency_key, request_sha256, material_version,
			manifest_sha256, language, source_code, created_at)
		SELECT lpad(i::text, 32, '0'), CASE WHEN i = 24 THEN $2 ELSE $1 END,
			CASE WHEN i = 23 THEN 'other-context' ELSE $3 END,
			'synthetic-history-key-' || i::text, repeat('b', 64), $4, repeat('a', 64), 'gnu++17',
			'synthetic-history-private-source', '2026-10-25T12:34:56.123456Z'::timestamptz
		FROM generate_series(1, 24) AS i RETURNING id
	)
	INSERT INTO judge_tasks (submission_id, state, verdict)
	SELECT id, 'finished', CASE WHEN right(id, 1) = '2' THEN 'WA' ELSE 'AC' END FROM accepted`,
		subject, other, submission.PersonalTrainingScope, version)
	must(t, err)
	must(t, tx.Commit(f.ctx))
}

func historyGET(e *loginEndpoint, suffix, subject string, cookies []*http.Cookie) loginResponse {
	return loginCall(e, entryTrainingHistory+suffix, "", cookies, func(r *http.Request) {
		r.Method = http.MethodGet
		if subject != "" {
			r.Header.Set("X-OJ-Expected-Subject", subject)
		}
	})
}

type historyResponse struct {
	Submissions []submission.Receipt `json:"submissions"`
	NextCursor  string               `json:"next_cursor"`
}

func readHistoryResponse(t *testing.T, response loginResponse) historyResponse {
	t.Helper()
	loginStatus(t, response, 200)
	if len(response.cookies) != 0 || response.headers.Get("Content-Type") != "application/json" {
		t.Fatal("history changed the session or response media type")
	}
	var page historyResponse
	must(t, json.Unmarshal(response.body, &page))
	if page.Submissions == nil {
		t.Fatal("missing history array")
	}
	var fields map[string]json.RawMessage
	must(t, json.Unmarshal(response.body, &fields))
	if len(fields) != 2 || fields["next_cursor"] == nil {
		t.Fatal("history response has an unexpected shape")
	}
	var rows []map[string]json.RawMessage
	must(t, json.Unmarshal(fields["submissions"], &rows))
	for _, row := range rows {
		if len(row) != 6 || row["id"] == nil || row["material_version"] == nil || row["language"] == nil ||
			row["created_at"] == nil || row["state"] == nil || row["verdict"] == nil {
			t.Fatal("history disclosed data beyond the projected summary or omitted a summary field")
		}
	}
	return page
}

func testEntryTrainingHistory(t *testing.T, f *fixture, binary string) {
	c := runtimeConfig(t, f, namespace(t), true)
	ca := testpki.New(t)
	start := func() *loginEndpoint {
		t.Helper()
		p := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
		return entryClient(t, p, ca)
	}
	disabled := start()
	c.TrainingHistory = true
	api := start()
	subject, other, empty := f.subject(t, false), f.subject(t, false), f.subject(t, false)
	for _, p := range []identity.Personal{subject, other, empty} {
		setHistoryEmailVerified(t, f, p.ID, true)
	}
	seedTrainingHistory(t, f, subject.ID, other.ID)
	login := func(p identity.Personal) []*http.Cookie {
		t.Helper()
		pending := beginLogin(t, api, p)
		c := completedCredential(t, loginCall(api, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
		return []*http.Cookie{{Name: transport.CookieName, Value: c.Value()}}
	}
	ownerCookie := login(subject)
	otherCookie := login(other)
	emptyCookie := login(empty)
	loginStatus(t, historyGET(disabled, "", subject.ID, ownerCookie), 404)
	loginStatus(t, historyGET(api, "", subject.ID, nil), 401)
	loginStatus(t, historyGET(api, "", "", ownerCookie), 400)
	loginStatus(t, historyGET(api, "", other.ID, ownerCookie), 409)
	loginStatus(t, loginCall(api, entryTrainingHistory, "{}", ownerCookie, nil), 405)
	for _, suffix := range []string{"?scope=other-context", "?subject=" + other.ID, "?cursor=", "?cursor=invalid", "?cursor=x&cursor=y"} {
		loginStatus(t, historyGET(api, suffix, subject.ID, ownerCookie), 400)
	}
	first := readHistoryResponse(t, historyGET(api, "", subject.ID, ownerCookie))
	if len(first.Submissions) != 20 || first.NextCursor == "" {
		t.Fatal("first history page has wrong size or missing continuation")
	}
	second := readHistoryResponse(t, historyGET(api, "?cursor="+first.NextCursor, subject.ID, ownerCookie))
	if len(second.Submissions) != 2 || second.NextCursor != "" {
		t.Fatal("last history page has wrong size or a continuation")
	}
	for i, row := range slices.Concat(first.Submissions, second.Submissions) {
		if row.ID != fmt.Sprintf("%032d", 22-i) || row.CreatedAt.Nanosecond() != 123456000 ||
			row.MaterialVersion != "synthetic-history-material" || row.Language != "gnu++17" ||
			row.State != "finished" || row.Verdict == nil {
			t.Fatal("seek order, owner/context isolation, microseconds, or summary changed")
		}
	}
	if page := readHistoryResponse(t, historyGET(api, "", other.ID, otherCookie)); len(page.Submissions) != 1 ||
		page.Submissions[0].ID != fmt.Sprintf("%032d", 24) || page.NextCursor != "" {
		t.Fatal("another user did not receive only their own history")
	}
	if page := readHistoryResponse(t, historyGET(api, "?cursor="+first.NextCursor, other.ID, otherCookie)); len(page.Submissions) != 0 || page.NextCursor != "" {
		t.Fatal("a cursor must not transfer the source owner's history")
	}
	if page := readHistoryResponse(t, historyGET(api, "", empty.ID, emptyCookie)); len(page.Submissions) != 0 || page.NextCursor != "" {
		t.Fatal("empty account has history")
	}
	setHistoryEmailVerified(t, f, subject.ID, false)
	loginStatus(t, historyGET(api, "?cursor="+first.NextCursor, subject.ID, ownerCookie), 403)
	loginStatus(t, loginCall(api, "/api/v1/session/logout", "", ownerCookie, nil), 204)
	loginStatus(t, historyGET(api, "", subject.ID, ownerCookie), 401)
}

func setHistoryEmailVerified(t *testing.T, f *fixture, subject string, verified bool) {
	t.Helper()
	_, err := f.pools[0].Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=$2 WHERE subject_id=$1`, subject, verified)
	must(t, err)
}
