//go:build integration && linux

package integration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type observedDraftSolvers struct {
	*materialstore.Local
	opened []materialstore.Object
}

func (s *observedDraftSolvers) Open(ctx context.Context, object materialstore.Object) (*materialstore.Snapshot, error) {
	s.opened = append(s.opened, object)
	return s.Local.Open(ctx, object)
}

func TestPrivateDraftConfiguredConvolutionEvidence(t *testing.T) {
	directory := os.Getenv("OJ_PRIVATE_DRAFT_BUNDLE_DIR")
	if directory == "" {
		t.Skip("requires the preverified private Library Checker bundle")
	}
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	lock, err := os.ReadFile("../../config/problem-sources/library-checker.json")
	must(t, err)
	p, err := problem.LoadBundle(directory, problemvalidation.ConvolutionSourceID, lock)
	must(t, err)
	t.Cleanup(func() { must(t, p.Close()) })
	expected, err := p.ImportIdentity()
	must(t, err)

	root := filepath.Join(t.TempDir(), "objects")
	must(t, os.Mkdir(root, 0700))
	config := materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1}
	written, err := materialstore.OpenLocal(config)
	must(t, err)
	t.Cleanup(func() { must(t, written.Close()) })
	sessions := fixedDraftSession{view: redisstore.View{SubjectID: f.actor.SubjectID, AuthGeneration: f.actor.AuthGeneration, RecoveryID: f.actor.RecoveryID}}
	service, err := problem.New(sessions, f.repository, written)
	must(t, err)
	credential, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	must(t, err)
	created, err := service.Create(ctx, credential, "convolution-evidence-import", p)
	must(t, err)
	must(t, p.Close())
	must(t, written.Close())

	reader, err := problempg.New(f.b, f.access)
	must(t, err)
	persisted, err := reader.GetOwn(ctx, f.actor, created.ID)
	must(t, err)
	reopened, err := materialstore.OpenLocal(config)
	must(t, err)
	t.Cleanup(func() { must(t, reopened.Close()) })
	objects := &observedDraftSolvers{Local: reopened}
	untrusted := expected
	untrusted.SourceLockSHA256 = strings.Repeat("0", 64)
	if _, err := problemvalidation.FreezeDraftConvolution(ctx, persisted, created.ID, f.actor.SubjectID, untrusted, objects, lock); err == nil || len(objects.opened) != 0 {
		t.Fatal("unreviewed source lock reached object storage")
	}
	if _, err := problemvalidation.FreezeDraftConvolution(ctx, persisted, created.ID, identityID(1101), expected, objects, lock); !errors.Is(err, problem.ErrInvalid) || len(objects.opened) != 0 {
		t.Fatal("wrong owner reached object storage", err)
	}
	sources, err := problemvalidation.FreezeDraftConvolution(ctx, persisted, created.ID, f.actor.SubjectID, expected, objects, lock)
	must(t, err)
	report := problemvalidation.NewConvolutionReport(sources)
	wantBinding := problemvalidation.ConvolutionDraftBinding{ID: created.ID, Version: expected.Version, ImportRevision: expected.ImportRevision}
	if report.Draft == nil || *report.Draft != wantBinding || report.SourceLock != expected.SourceLockSHA256 ||
		report.Status != "incomplete" || report.StopConfirmed || report.Publication != "blocked" {
		t.Fatal("evidence lost its draft provenance or implied validation")
	}
	wantObjects := []materialstore.Object{
		{SHA256: report.Oracle.SHA256, Size: report.Oracle.Size},
		{SHA256: report.Candidate.SHA256, Size: report.Candidate.Size},
	}
	if !slices.Equal(objects.opened, wantObjects) {
		t.Fatal("persisted preparation did not read exactly the two reviewed roles")
	}
	raw, err := json.Marshal(report)
	must(t, err)
	if strings.Contains(string(raw), f.actor.SubjectID) {
		t.Fatal("report contained private owner identity")
	}
	again, err := reader.GetOwn(ctx, f.actor, created.ID)
	must(t, err)
	if again.State != "private-draft" || again.ValidationState != "not-run" || again.ImportRevision != expected.ImportRevision {
		t.Fatal("preparation altered the persisted draft")
	}
}
