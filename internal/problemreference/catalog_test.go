package problemreference

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func preparedCatalogFixture(t *testing.T, b *problemfixture.Bundle) *problem.Prepared {
	t.Helper()
	b.Write(t)
	source := b.Document["provenance"].(map[string]any)["source_id"].(string)
	p, err := problem.LoadBundle(b.Directory, source, b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	return p
}

func draftSelection(t *testing.T, p *problem.Prepared) problem.DraftIdentity {
	t.Helper()
	i, err := p.ImportIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return problem.DraftIdentity{SourceID: i.SourceID, Version: i.Version, ImportRevision: i.ImportRevision}
}

func historicalSpecification(raw []byte) Specification {
	return Specification{Identity: problem.ImportIdentity{SourceID: "library-checker:retired_source",
		Version: strings.Repeat("a", 64), ImportRevision: strings.Repeat("b", 64), SourceLockSHA256: strings.Repeat("c", 64)},
		Protocol: IdentityProtocol, ProfileJSON: raw, ProfileSHA256: profileDigest(raw)}
}

func TestCatalogFreezesTrustedMetadataAndReturnsDetachedBytes(t *testing.T) {
	b := problemfixture.New(t)
	p := preparedCatalogFixture(t, b)
	selection := draftSelection(t, p)
	sources := map[string]*problem.Prepared{selection.SourceID: p}
	catalog, err := NewCatalog(sources)
	if err != nil {
		t.Fatal(err)
	}
	delete(sources, selection.SourceID)
	spec, err := catalog.Select(selection)
	profile, _, resolveErr := problemvalidation.ResolveSource(selection.SourceID)
	raw, marshalErr := json.Marshal(profile)
	wantIdentity, identityErr := p.ImportIdentity()
	if err != nil || resolveErr != nil || marshalErr != nil || identityErr != nil || !spec.Valid() || spec.Identity != wantIdentity || !bytes.Equal(spec.ProfileJSON, raw) || spec.ProfileSHA256 != profile.Digest() {
		t.Fatal("catalog did not capture exact trusted import and compact profile", err, resolveErr, marshalErr, identityErr)
	}
	clone := spec.Clone()
	clone.ProfileJSON[0] = '!'
	if !spec.Valid() || clone.Valid() {
		t.Fatal("Clone aliases profile bytes or ignores corruption")
	}
	spec.ProfileJSON[0] = '!'
	again, err := catalog.Select(selection)
	if err != nil || !again.Valid() || !bytes.Equal(again.ProfileJSON, raw) {
		t.Fatal("selection mutated immutable catalog", err)
	}
	for _, field := range []string{"source", "version", "revision"} {
		changed := selection
		switch field {
		case "source":
			changed.SourceID = "library-checker:unionfind"
		case "version":
			changed.Version = strings.Repeat("d", 64)
		case "revision":
			changed.ImportRevision = strings.Repeat("e", 64)
		}
		if got, err := catalog.Select(changed); !errors.Is(err, ErrSourceChanged) || got.Valid() {
			t.Fatal("catalog borrowed another import", field, err)
		}
	}
}

func TestCatalogMetadataSurvivesPreparationCloseAndMaterialRemoval(t *testing.T) {
	b := problemfixture.New(t)
	p := preparedCatalogFixture(t, b)
	selection := draftSelection(t, p)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(b.Directory); err != nil {
		t.Fatal(err)
	}
	// The fixture's own directory is gone: both construction and selection can
	// succeed only from the metadata frozen before closure, without material I/O.
	catalog, err := NewCatalog(map[string]*problem.Prepared{selection.SourceID: p})
	if err != nil {
		t.Fatal("closed preparation metadata was unavailable", err)
	}
	if spec, err := catalog.Select(selection); err != nil || !spec.Valid() {
		t.Fatal("metadata selection accessed removed materials", err)
	}
	retired, err := NewCatalog(nil)
	if err != nil {
		t.Fatal("empty retired catalog rejected", err)
	}
	if _, err := retired.Select(selection); !errors.Is(err, ErrSourceChanged) {
		t.Fatal("retired source selected", err)
	}
	var absent *Catalog
	if _, err := absent.Select(selection); !errors.Is(err, ErrSourceChanged) {
		t.Fatal("nil catalog selected a source")
	}
}

func TestCatalogSourceLockConsistencyAndInvalidPreparations(t *testing.T) {
	a, b := problemfixture.New(t), problemfixture.NewUnionFind(t)
	p, q := preparedCatalogFixture(t, a), preparedCatalogFixture(t, b)
	if _, err := NewCatalog(map[string]*problem.Prepared{"library-checker:aplusb": p, "library-checker:unionfind": q}); !errors.Is(err, ErrInvalid) {
		t.Fatal("mixed trusted source locks accepted", err)
	}
	for _, sources := range []map[string]*problem.Prepared{
		{"library-checker:aplusb": nil}, {"library-checker:aplusb": {}}, {"library-checker:unionfind": p},
	} {
		if _, err := NewCatalog(sources); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid configuration accepted", err)
		}
	}
	oversize := map[string]*problem.Prepared{}
	for i := 0; i < 11; i++ {
		oversize[strings.Repeat("x", i+1)] = p
	}
	if _, err := NewCatalog(oversize); !errors.Is(err, ErrInvalid) {
		t.Fatal("unbounded source catalog accepted", err)
	}
	// Two independently loaded drafts can share one explicitly trusted lock.
	var common map[string]any
	if err := json.Unmarshal(a.Lock, &common); err != nil {
		t.Fatal(err)
	}
	var other map[string]any
	if err := json.Unmarshal(b.Lock, &other); err != nil {
		t.Fatal(err)
	}
	common["problems"] = append(common["problems"].([]any), other["problems"].([]any)...)
	lock := problemfixture.JSON(t, common)
	for _, bundle := range []*problemfixture.Bundle{a, b} {
		bundle.Lock = bytes.Clone(lock)
		bundle.Files["provenance/source-lock.json"] = bytes.Clone(lock)
		bundle.Document["provenance"].(map[string]any)["source_lock_sha256"] = problemfixture.Hash(lock)
		bundle.Write(t)
	}
	p, q = preparedCatalogFixture(t, a), preparedCatalogFixture(t, b)
	c, err := NewCatalog(map[string]*problem.Prepared{"library-checker:aplusb": p, "library-checker:unionfind": q})
	if err != nil {
		t.Fatal("shared trusted lock rejected", err)
	}
	for _, prepared := range []*problem.Prepared{p, q} {
		if spec, err := c.Select(draftSelection(t, prepared)); err != nil || spec.Identity.SourceLockSHA256 != problemfixture.Hash(lock) {
			t.Fatal("source lock not frozen", err)
		}
	}
}

func TestCatalogRejectsLoadedButUnresolvedSource(t *testing.T) {
	b := problemfixture.New(t)
	var lock map[string]any
	if err := json.Unmarshal(b.Lock, &lock); err != nil {
		t.Fatal(err)
	}
	lock["problems"].([]any)[0].(map[string]any)["path"] = "sample/retired_source"
	b.Lock = problemfixture.JSON(t, lock)
	b.Files["provenance/source-lock.json"] = b.Lock
	provenance := b.Document["provenance"].(map[string]any)
	provenance["source_id"], provenance["source_path"], provenance["source_lock_sha256"] = "library-checker:retired_source", "sample/retired_source", problemfixture.Hash(b.Lock)
	b.Write(t)
	p := preparedCatalogFixture(t, b)
	if _, err := NewCatalog(map[string]*problem.Prepared{"library-checker:retired_source": p}); !errors.Is(err, ErrInvalid) {
		t.Fatal("unresolved profile admitted", err)
	}
}

func TestStoredSpecificationValidationDoesNotResolveCurrentProfiles(t *testing.T) {
	good := historicalSpecification([]byte(`{"retired":true,"id":"old-profile"}`))
	if !good.Valid() {
		t.Fatal("historical self-consistent configuration rejected")
	}
	exact := historicalSpecification([]byte(`{"legacy":"` + strings.Repeat("x", MaxProfileBytes-len(`{"legacy":""}`)) + `"}`))
	if len(exact.ProfileJSON) != MaxProfileBytes || !exact.Valid() {
		t.Fatal("exact configuration byte cap rejected")
	}
	for _, raw := range [][]byte{nil, []byte(`null`), []byte(`{}`), []byte(`[]`), []byte(`true`), []byte(`{"x":`), []byte("{\"x\":\"\xff\"}"), append(bytes.Clone(exact.ProfileJSON), ' ')} {
		if historicalSpecification(raw).Valid() {
			t.Fatal("invalid JSON shape/byte bounds accepted")
		}
	}
	for _, mutate := range []func(*Specification){
		func(s *Specification) { s.Identity.SourceID = "bad source" },
		func(s *Specification) { s.Identity.Version = "short" },
		func(s *Specification) { s.Identity.ImportRevision = strings.Repeat("G", 64) },
		func(s *Specification) { s.Identity.SourceLockSHA256 = "" },
		func(s *Specification) { s.Protocol = "unknown-protocol" },
		func(s *Specification) { s.ProfileSHA256 = strings.Repeat("0", 64) },
		func(s *Specification) { s.ProfileJSON[0] = '!' },
	} {
		changed := good.Clone()
		mutate(&changed)
		if changed.Valid() {
			t.Fatal("invalid stored specification accepted")
		}
	}
}
