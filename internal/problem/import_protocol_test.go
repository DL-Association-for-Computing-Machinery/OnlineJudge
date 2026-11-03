package problem

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

type protocolRepository struct {
	*serviceRepository
	finalCheck error
	onCheck    func(int)
	receipts   int
	result     ImportReceipt
}

func (r *protocolRepository) CheckAuthor(ctx context.Context, actor identity.ProblemActor) error {
	err := r.serviceRepository.CheckAuthor(ctx, actor)
	if r.onCheck != nil {
		r.onCheck(r.checks)
	}
	if r.checks > 1 && r.finalCheck != nil {
		return r.finalCheck
	}
	return err
}
func (r *protocolRepository) Receipt(context.Context, identity.ProblemActor, string) (ImportReceipt, error) {
	r.receipts++
	return r.result, r.errorFinal
}

func protocolFixture(t *testing.T, prepared *Prepared) (*Importer, *sequencedSessions, *protocolRepository, redisstore.Credential) {
	t.Helper()
	if prepared == nil {
		b := problemfixture.New(t)
		var err error
		prepared, err = LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { closePrepared(t, prepared) })
	}
	credential, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	view := redisstore.View{SubjectID: strings.Repeat("1", 32), AuthGeneration: 1, RecoveryID: strings.Repeat("2", 32)}
	sessions := &sequencedSessions{credential: credential}
	for range 4 {
		sessions.observations = append(sessions.observations, sessionObservation{view: view})
	}
	repo := &protocolRepository{serviceRepository: &serviceRepository{materials: &checkingMaterials{}}, result: ImportReceipt{DraftID: strings.Repeat("3", 32), RequestKey: "original-receipt-request", ImportRevision: strings.Repeat("a", 64)}}
	service, err := New(sessions, repo, repo.materials)
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewImporter(service, map[string]*Prepared{prepared.content.SourceID: prepared}, 1)
	if err != nil {
		t.Fatal(err)
	}
	return importer, sessions, repo, credential
}

func TestSubjectPinPrecedesAllBusinessIO(t *testing.T) {
	for _, op := range []string{"sources", "import", "receipt"} {
		t.Run(op, func(t *testing.T) {
			i, sessions, repo, credential := protocolFixture(t, nil)
			expected := strings.Repeat("a", 32) // Actual session belongs to another subject.
			var err error
			switch op {
			case "sources":
				_, err = i.SourcesForSubject(t.Context(), credential, expected)
			case "import":
				_, err = i.ImportPinnedForSubject(t.Context(), credential, "original-receipt-request", "unknown", strings.Repeat("f", 64), expected)
			case "receipt":
				_, err = i.ReceiptForSubject(t.Context(), credential, "original-receipt-request", expected)
			}
			if !errors.Is(err, ErrSubjectChanged) || sessions.calls != 1 || repo.checks != 0 || repo.receipts != 0 || repo.creates != 0 || repo.materials.puts != 0 || len(i.slots) != 0 {
				t.Fatalf("subject mismatch reached business I/O: %v", err)
			}
		})
	}
}

func TestSubjectPinMatchingUsesExistingFinalAuthority(t *testing.T) {
	for _, op := range []string{"sources", "import", "receipt"} {
		t.Run(op, func(t *testing.T) {
			i, sessions, repo, credential := protocolFixture(t, nil)
			expected := sessions.observations[0].view.SubjectID
			var err error
			switch op {
			case "sources":
				_, err = i.SourcesForSubject(t.Context(), credential, expected)
				if sessions.calls != 2 || repo.checks != 2 {
					t.Fatal("source omitted final authority")
				}
			case "import":
				content := i.sources["library-checker:aplusb"].Content()
				_, err = i.ImportPinnedForSubject(t.Context(), credential, "original-receipt-request", content.SourceID, content.RequestSHA256, expected)
				if sessions.calls != 2 || repo.checks != 1 || repo.creates != 1 || repo.materials.puts == 0 {
					t.Fatal("import omitted final authority")
				}
			case "receipt":
				_, err = i.ReceiptForSubject(t.Context(), credential, "original-receipt-request", expected)
				if sessions.calls != 1 || repo.receipts != 1 {
					t.Fatal("receipt omitted owner-scoped read")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSourceCatalogFinalAuthorityAndDetachedSummary(t *testing.T) {
	for _, mode := range []string{"success", "initial-session", "initial-role", "final-session", "actor-changed", "final-role", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			i, sessions, repo, credential := protocolFixture(t, nil)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := error(nil)
			switch mode {
			case "initial-session":
				want = redisstore.ErrStale
				sessions.observations[0].err = want
			case "initial-role":
				want = identity.ErrForbidden
				repo.preflight = want
			case "final-session":
				want = redisstore.ErrStale
				sessions.observations[1].err = want
			case "actor-changed":
				want = identity.ErrConflict
				sessions.observations[1].view.AuthGeneration++
			case "final-role":
				want = identity.ErrForbidden
				repo.finalCheck = want
			case "canceled":
				want = context.Canceled
				repo.onCheck = func(n int) {
					if n == 2 {
						cancel()
					}
				}
			}
			// Catalog and receipt reads do not compete with material work.
			i.slots <- struct{}{}
			sources, err := i.Sources(ctx, credential)
			<-i.slots
			if !errors.Is(err, want) || (want != nil && sources != nil) {
				t.Fatalf("partial catalog or wrong refusal: %v", err)
			}
			if repo.materials.puts != 0 || repo.creates != 0 || repo.receipts != 0 {
				t.Fatal("catalog accessed private material or import state")
			}
			if want == nil {
				content := i.sources["library-checker:aplusb"].Content()
				expected := []ImportSource{{SourceID: content.SourceID, Title: "Synthetic A + B", Version: content.Version, ImportRevision: content.RequestSHA256}}
				if !reflect.DeepEqual(sources, expected) {
					t.Fatal("catalog omitted complete candidate identity")
				}
				sources[0].Title = "caller mutation"
				again, e := i.Sources(ctx, credential)
				if e != nil || !reflect.DeepEqual(again, expected) {
					t.Fatal("caller changed frozen catalog")
				}
			}
		})
	}
}

func TestPinnedImportBindsEvidenceBeyondDocumentVersion(t *testing.T) {
	bundle := problemfixture.New(t)
	before, err := LoadBundle(bundle.Directory, "library-checker:aplusb", bundle.Lock)
	if err != nil {
		t.Fatal(err)
	}
	defer closePrepared(t, before)
	old := before.Content()
	bundle.Files["provenance/source-validation.json"] = []byte("{\"changed-evidence\":true}\n")
	bundle.Write(t)
	after, err := LoadBundle(bundle.Directory, "library-checker:aplusb", bundle.Lock)
	if err != nil {
		t.Fatal(err)
	}
	defer closePrepared(t, after)
	current := after.Content()
	if old.Version != current.Version || old.RequestSHA256 == current.RequestSHA256 {
		t.Fatal("fixture did not change only full import identity")
	}
	i, sessions, repo, credential := protocolFixture(t, after)
	i.slots <- struct{}{} // Revision mismatch wins before even acquiring capacity.
	result, err := i.ImportPinned(t.Context(), credential, "never-committed-original-key", current.SourceID, old.RequestSHA256)
	<-i.slots
	if !errors.Is(err, ErrSourceChanged) || result != (ImportReceipt{}) || repo.materials.puts != 0 || repo.creates != 0 || sessions.calls != 1 {
		t.Fatal("old revision admitted changed evidence or material I/O", err)
	}
	repo.checks = 0
	result, err = i.ImportPinned(t.Context(), credential, "pinned-current-original-key", current.SourceID, current.RequestSHA256)
	if err != nil || result.ImportRevision != current.RequestSHA256 || result.Version != current.Version || result.SourceID != current.SourceID || repo.creates != 1 || repo.materials.puts == 0 {
		t.Fatal("exact current revision failed", err)
	}
}

func TestPinnedImportDoesNotExposeCandidateBeforeAuthority(t *testing.T) {
	for _, mode := range []string{"session", "role", "authorized"} {
		for _, source := range []string{"library-checker:aplusb", "unknown"} {
			t.Run(mode+"/"+source, func(t *testing.T) {
				i, sessions, repo, credential := protocolFixture(t, nil)
				want := ErrSourceChanged
				if source == "unknown" {
					want = ErrUnknownSource
				}
				if mode == "session" {
					want = redisstore.ErrStale
					sessions.observations[0].err = want
				}
				if mode == "role" {
					want = identity.ErrForbidden
					repo.preflight = want
				}
				_, err := i.ImportPinned(t.Context(), credential, "pinned-authorization-original-key", source, strings.Repeat("f", 64))
				if !errors.Is(err, want) || repo.materials.puts != 0 || repo.creates != 0 {
					t.Fatal("candidate observed before current authority", err)
				}
			})
		}
	}
}

func TestReceiptNeedsFreshSessionButNotCandidateOrMaterialSlot(t *testing.T) {
	i, sessions, repo, credential := protocolFixture(t, nil)
	delete(i.sources, "library-checker:aplusb")
	i.slots <- struct{}{}
	result, err := i.Receipt(t.Context(), credential, repo.result.RequestKey)
	<-i.slots
	if err != nil || result != repo.result || sessions.calls != 1 || repo.receipts != 1 || repo.materials.puts != 0 || repo.creates != 0 {
		t.Fatal("receipt depended on catalog/materials", err)
	}
	sessions.observations[1].err = redisstore.ErrStale
	result, err = i.Receipt(t.Context(), credential, repo.result.RequestKey)
	if !errors.Is(err, redisstore.ErrStale) || result != (ImportReceipt{}) || repo.receipts != 1 {
		t.Fatal("stale credential reached receipt", err)
	}
}
