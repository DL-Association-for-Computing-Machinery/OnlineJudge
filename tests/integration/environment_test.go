//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPinnedMaterials(t *testing.T, ctx context.Context, pool *pgxpool.Pool, client *http.Client, pinnedURL, legacyURL string, legacy *postgres.Store) {
	version, manifest, err := experiment.SelectFixture(experiment.PinnedVersion)
	must(t, err)
	store, err := postgres.New(pool, version, manifest)
	must(t, err)
	request := submission.Request{MaterialVersion: version, Language: "gnu++17", Source: "// synthetic pinned snapshot; not executed\n"}
	_, _, err = store.Accept(ctx, "environment", "m0-training", "pinned-environment-01", request)
	if !errors.Is(err, submission.ErrMaterial) {
		t.Fatal("unregistered v2 accepted")
	}
	must(t, store.RegisterFixture(ctx))
	must(t, store.RegisterFixture(ctx))
	row, _, err := store.Accept(ctx, "environment", "m0-training", "pinned-environment-01", request)
	must(t, err)
	_, _, err = legacy.Accept(ctx, "environment", "m0-training", "pinned-environment-01", request)
	if !errors.Is(err, submission.ErrMaterial) {
		t.Fatal("legacy controller accepted v2")
	}
	status, _, _, err := workerRequest(client, legacyURL, "claim", "{}")
	must(t, err)
	if status != 204 {
		t.Fatalf("legacy controller claimed new task: %d", status)
	}
	status, data, _, err := workerRequest(client, pinnedURL, "claim", "{}")
	must(t, err)
	if status != 200 {
		t.Fatalf("v2 claim: %d %s", status, data)
	}
	var lease struct {
		Token        string `json:"lease_token"`
		SubmissionID string `json:"submission_id"`
	}
	must(t, json.Unmarshal(data, &lease))
	if lease.SubmissionID != row.ID {
		t.Fatal("wrong submission claimed")
	}
	body, _ := json.Marshal(map[string]string{"lease_token": lease.Token})
	status, data, _, err = workerRequest(client, pinnedURL, "assignment", string(body))
	must(t, err)
	if status != 200 {
		t.Fatalf("v2 assignment: %d %s", status, data)
	}
	var descriptor materials.Descriptor
	must(t, json.Unmarshal(data, &descriptor))
	if !bytes.Equal(descriptor.Manifest, manifest) || descriptor.ManifestSHA256 != materialHash(manifest) || descriptor.Source != request.Source {
		t.Fatal("pinned snapshot or hash changed in transport")
	}
	status, data, _, err = workerRequest(client, pinnedURL, "material", materialBody(lease.Token))
	must(t, err)
	if status != 200 || string(data) != "1 2\n" {
		t.Fatal("v2 blob download failed")
	}
	status, _, _, err = workerRequest(client, legacyURL, "assignment", string(body))
	must(t, err)
	if status != 503 {
		t.Fatalf("legacy controller relabeled new lease: %d", status)
	}
	changed := bytes.Replace(manifest, []byte("15.3.0"), []byte("99.0.0"), 1)
	conflicting, err := postgres.New(pool, version, changed)
	must(t, err)
	if !errors.Is(conflicting.RegisterFixture(ctx), submission.ErrMaterial) {
		t.Fatal("profile overwritten")
	}
	_, err = pool.Exec(ctx, `UPDATE submission_materials SET manifest=$2::jsonb WHERE version_id=$1`, version, changed)
	must(t, err)
	status, _, _, readErr := workerRequest(client, pinnedURL, "assignment", string(body))
	_, restoreErr := pool.Exec(ctx, `UPDATE submission_materials SET manifest=$2::jsonb WHERE version_id=$1`, version, manifest)
	must(t, restoreErr)
	must(t, readErr)
	if status != 503 {
		t.Fatal("tampered environment served")
	}
	// Re-registering legacy bytes must still succeed after v2 registration/tampering.
	must(t, legacy.RegisterFixture(ctx))
}
