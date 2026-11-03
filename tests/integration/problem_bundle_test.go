//go:build integration

package integration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func TestPrivateDraftMigration(t *testing.T) {
	f := newDraftFixture(t)
	_, err := f.provider.Down(f.ctx)
	must(t, err)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("current application accepted schema6")
	}
	var count int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM identity_credentials`).Scan(&count))
	if count != 2 {
		t.Fatal("empty draft rollback changed accounts")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM identity_problem_authors`).Scan(&count))
	if count != 0 {
		t.Fatal("migration automatically granted existing accounts")
	}
	f.role(t, f.actor.SubjectID, 0, false)
	if _, err = f.provider.Down(f.ctx); err == nil {
		t.Fatal("rollback discarded explicit disabled role and audit")
	}
}

// This manually selected test imports the actual prebuilt, private 10-problem
// bundle. It is not replaced by a tiny CI fixture or source execution claim.
func TestPrivateDraftConfiguredBundle(t *testing.T) {
	directory := os.Getenv("OJ_PRIVATE_DRAFT_BUNDLE_DIR")
	root := os.Getenv("OJ_PRIVATE_DRAFT_OBJECT_ROOT")
	if directory == "" || root == "" {
		t.Skip("manual real-bundle acceptance requires private source snapshot and fresh preprovisioned object root")
	}
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	// Full payload hashing has an explicit acceptance budget, independently of
	// the short synthetic fixture setup and each 5s metadata transaction.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	f.ctx = ctx
	lock, err := os.ReadFile("../../config/problem-sources/library-checker.json")
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(directory, "index.json"))
	must(t, err)
	var index struct {
		Problems []struct {
			SourceID string `json:"source_id"`
		} `json:"problems"`
	}
	must(t, json.Unmarshal(raw, &index))
	if len(index.Problems) != 10 {
		t.Fatal("actual configured corpus not ten problems")
	}
	materials, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 2})
	must(t, err)
	sessions := fixedDraftSession{redisstore.View{SubjectID: f.actor.SubjectID, AuthGeneration: 1, RecoveryID: f.actor.RecoveryID}}
	service, err := problem.New(sessions, f.repository, materials)
	must(t, err)
	c, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	must(t, err)
	var drafts []problem.Draft
	cases, examples := 0, 0
	var previewBytes int
	for _, entry := range index.Problems {
		prepared, e := problem.LoadBundle(directory, entry.SourceID, lock)
		must(t, e)
		content := prepared.Content()
		var doc struct {
			Title     string `json:"title"`
			Statement struct {
				Path string `json:"path"`
			} `json:"statement"`
			Cases []struct {
				ID     string `json:"id"`
				Sample bool   `json:"sample"`
				Input  struct {
					Path string `json:"path"`
				} `json:"input"`
				Answer struct {
					Path string `json:"path"`
				} `json:"answer"`
			} `json:"test_cases"`
			Examples []string `json:"examples"`
		}
		must(t, json.Unmarshal(content.Document, &doc))
		cases += len(doc.Cases)
		examples += len(doc.Examples)
		draft, e := service.Create(f.ctx, c, "configured-corpus-"+entry.SourceID[len("library-checker:"):], prepared)
		must(t, e)
		importer, e := problem.NewImporter(service, map[string]*problem.Prepared{entry.SourceID: prepared}, 1)
		must(t, e)
		preview, e := importer.Preview(f.ctx, c, draft.ID)
		must(t, e)
		statement, e := os.ReadFile(filepath.Join(directory, doc.Statement.Path))
		must(t, e)
		if preview.DraftID != draft.ID || preview.Version != draft.Version || preview.Title != doc.Title || preview.Language != "en" ||
			preview.Markdown != string(statement) || len(preview.Examples) != len(doc.Examples) {
			t.Fatal("actual corpus preview changed statement, version or sample count")
		}
		previewBytes += len(preview.Markdown)
		for n, exampleID := range doc.Examples {
			matched := false
			for _, point := range doc.Cases {
				if point.ID != exampleID || !point.Sample {
					continue
				}
				input, e := os.ReadFile(filepath.Join(directory, point.Input.Path))
				must(t, e)
				answer, e := os.ReadFile(filepath.Join(directory, point.Answer.Path))
				must(t, e)
				got := preview.Examples[n]
				if got.ID != exampleID || got.Input != string(input) || got.Output != string(answer) {
					t.Fatal("actual corpus preview changed sample order or exact bytes")
				}
				previewBytes += len(got.Input) + len(got.Output)
				matched = true
			}
			if !matched {
				t.Fatal("actual corpus preview included a non-sample case")
			}
		}
		must(t, prepared.Close())
		drafts = append(drafts, draft)
	}
	must(t, materials.Close())
	materials, err = materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 2})
	must(t, err)
	defer func() { must(t, materials.Close()) }()
	repository, err := problempg.New(f.b, f.access)
	must(t, err)
	reopened, err := problem.New(sessions, repository, materials)
	must(t, err)
	unique := map[string]materialstore.Object{}
	references := 0
	for _, draft := range drafts {
		got, e := reopened.GetOwn(f.ctx, c, draft.ID)
		must(t, e)
		if got.Version != draft.Version || len(got.References) != len(draft.References) {
			t.Fatal("reopened configured draft changed")
		}
		for _, r := range got.References {
			references++
			unique[r.Object.SHA256] = r.Object
		}
	}
	var bytes int64
	for _, object := range unique {
		snapshot, e := materials.Open(f.ctx, object)
		must(t, e)
		must(t, snapshot.Close())
		bytes += object.Size
	}
	if cases != 254 || examples != 18 {
		t.Fatal("configured source case/example mapping changed")
	}
	t.Logf("configured ten drafts/254 cases/18 examples committed and exact statement/sample previews verified (%d raw bytes); %d logical references, %d immutable objects, %d bytes independently reopened after Store close; owner scope checked through new PG pool; session observation stubbed here", previewBytes, references, len(unique), bytes)
}
