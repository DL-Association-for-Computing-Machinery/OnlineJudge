//go:build integration

package integration_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func TestPublishedTrainingTransaction(t *testing.T) {
	f := newBacklogFixture(t)
	backlogPolicy(t, f, 4, 2)
	h := identityHasher(t, false)
	accounts, err := identitypg.New(f.a, h)
	must(t, err)
	epoch, err := accounts.BeginRecovery(f.ctx)
	must(t, err)
	must(t, accounts.FinishRecovery(f.ctx, epoch))
	email, err := identity.ParseEmail("publication-owner@example.invalid")
	must(t, err)
	person, err := accounts.InitializePersonal(f.ctx, identityID(1010), email, true, identityRecord(t, h, "synthetic published training password"))
	must(t, err)
	actor := identity.TrainingActor{SubjectID: person.ID, SessionVersion: identity.SessionVersion{AuthGeneration: person.AuthGeneration, RecoveryID: epoch}}
	const otherQueue = "second-published-training-fixture"
	raw := []byte(`{"queue":"second"}`)
	_, secondDigest, err := postgres.CanonicalManifest(raw)
	must(t, err)
	second, err := postgres.New(f.a, otherQueue, raw)
	must(t, err)
	must(t, second.RegisterFixture(f.ctx))
	var firstDigest string
	must(t, f.a.QueryRow(f.ctx, `SELECT manifest_sha256 FROM submission_materials WHERE version_id=$1`, f.request.MaterialVersion).Scan(&firstDigest))
	idA, idB := identityID(1011), identityID(1012)
	showA, showB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	const key = "multi-publication-same-key"
	lookups := 0
	resolve := func(_ context.Context, _ pgx.Tx, id, version string) (postgres.PublishedMaterial, error) {
		lookups++
		switch {
		case id == idA && version == showA:
			return postgres.PublishedMaterial{Version: f.request.MaterialVersion, Digest: firstDigest}, nil
		case id == idB && version == showB:
			return postgres.PublishedMaterial{Version: otherQueue, Digest: secondDigest}, nil
		default:
			return postgres.PublishedMaterial{}, submission.ErrNotFound
		}
	}
	submit := func(id, version, key string) trainingAcceptance {
		r, created, err := postgres.AcceptPublishedTraining(f.ctx, f.a, actor, key, id, version, f.request.Source, resolve)
		return trainingAcceptance{r, created, err}
	}
	if a := submit(idA, showA, key); !errors.Is(a.err, identity.ErrTrainingAccountRequired) || lookups != 0 {
		t.Fatalf("unverified account reached publication selection: %+v", a)
	}
	_, err = f.a.Exec(f.ctx, "UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1", person.ID)
	must(t, err)
	a := submit(idA, showA, key)
	must(t, a.err)
	replay := submit(idA, showA, key)
	must(t, replay.err)
	if !a.created || replay.created || replay.receipt.ID != a.receipt.ID || a.receipt.MaterialVersion != f.request.MaterialVersion {
		t.Fatal("original receipt or material changed")
	}
	if got := submit(idB, showA, "stale-publication-key"); !errors.Is(got.err, submission.ErrNotFound) {
		t.Fatalf("another publication's version was admitted: %+v", got)
	}
	if got := submit(idB, showB, key); !errors.Is(got.err, submission.ErrConflict) {
		t.Fatalf("same request key was reusable on another problem: %+v", got)
	}
	b := submit(idB, showB, "second-publication-key")
	must(t, b.err)
	if !b.created || b.receipt.MaterialVersion != otherQueue {
		t.Fatal("second publication did not route to the second queue")
	}
	if got := submit(idB, showB, "shared-backlog-policy-key"); !errors.Is(got.err, submission.ErrBacklog) {
		t.Fatalf("backlog was not shared by the queues: %+v", got)
	}
	backlogCounts(t, f, 2)
}
