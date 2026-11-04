//go:build integration

package integration_test

import (
	"encoding/base64"
	"os"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func TestReferenceRequestServiceWithoutMaterialIO(t *testing.T) {
	f := newReferenceRequestFixture(t)
	f.limits(t, 1, 1)
	credential, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	must(t, err)
	sessions := fixedDraftSession{view: redisstore.View{SubjectID: f.actor.SubjectID, AuthGeneration: f.actor.AuthGeneration, RecoveryID: f.actor.RecoveryID}}
	service, err := problemreference.New(sessions, f.store, f.catalog)
	must(t, err)
	// The test owns this fixture directory. Removing it after metadata admission
	// proves the actual service/PG path cannot be relying on live bundle files.
	must(t, f.prepared.Close())
	must(t, os.RemoveAll(f.bundle.Directory))
	first, err := service.Enqueue(f.ctx, credential, "reference-service-original-key", f.draft.ID)
	must(t, err)
	if first.Authorization.Actor != f.actor || first.Owner != f.actor.SubjectID || first.State != problemreference.Queued {
		t.Fatal("service/PG path did not bind observed session and fixed draft")
	}
	receipt, err := service.Receipt(f.ctx, credential, first.RequestKey)
	must(t, err)
	if !reflect.DeepEqual(first, receipt) {
		t.Fatal("service readback changed stored intent")
	}
	cancelled, err := service.Cancel(f.ctx, credential, first.RequestKey)
	must(t, err)
	if cancelled.State != problemreference.Cancelled || cancelled.Authorization != first.Authorization || !reflect.DeepEqual(cancelled.Specification, first.Specification) {
		t.Fatal("service cancellation changed original configuration or authority")
	}
	f.role(t, f.actor.SubjectID, 1, false)
	r, err := service.Receipt(f.ctx, credential, first.RequestKey)
	noReferenceResult(t, r, err, identity.ErrForbidden)
	f.counts(t, 1, 0)
	t.Log("actual request Service + PostgreSQL persisted/read/cancelled after private fixture removal; session observation is explicitly stubbed, not Redis/HTTP/Worker acceptance")
}
