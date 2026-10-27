package materials

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

type materialStoreFunc func(context.Context, submission.NodeIdentity, string) (submission.TaskMaterials, error)

func (f materialStoreFunc) Materials(c context.Context, n submission.NodeIdentity, token string) (submission.TaskMaterials, error) {
	return f(c, n, token)
}

type readerFunc func(context.Context, string) (*File, error)

func (f readerFunc) Open(c context.Context, h string) (*File, error) { return f(c, h) }

type trackedFile struct {
	io.Reader
	closed bool
}

func (f *trackedFile) Close() error { f.closed = true; return nil }

func testMaterials() submission.TaskMaterials {
	h := checksum([]byte("abc"))
	manifest := []byte(`{"version":"v1","language":"gnu++17","tests":[{"id":"01","input_sha256":"` + h + `","answer_sha256":"` + h + `"}]}`)
	return submission.TaskMaterials{SubmissionID: "s1", Version: "v1", Language: "gnu++17", Source: "// 源码😀\n", Manifest: manifest, ManifestSHA256: checksum(manifest)}
}

func TestServiceAuthorizationAndIntegrity(t *testing.T) {
	node := submission.NodeIdentity{ID: "n1", CredentialVersion: 1}
	for _, fail := range []string{"none", "first-auth", "revoked-during-read", "expired-during-read", "changed-source", "changed-manifest", "corrupt-manifest", "unknown-test", "path", "kind"} {
		t.Run(fail, func(t *testing.T) {
			calls, reads := 0, 0
			file := &trackedFile{Reader: strings.NewReader("abc")}
			store := materialStoreFunc(func(_ context.Context, n submission.NodeIdentity, token string) (submission.TaskMaterials, error) {
				calls++
				if n != node || token != "lease" {
					t.Fatal("identity/token not forwarded")
				}
				value := testMaterials()
				if fail == "first-auth" || (fail == "revoked-during-read" && calls == 2) {
					return value, submission.ErrNode
				}
				if fail == "expired-during-read" && calls == 2 {
					return value, submission.ErrLease
				}
				if fail == "changed-source" && calls == 2 {
					value.Source = "other"
				}
				if fail == "changed-manifest" && calls == 2 {
					value.Manifest = append(value.Manifest, ' ')
					value.ManifestSHA256 = checksum(value.Manifest)
				}
				if fail == "corrupt-manifest" {
					value.Manifest[0] = 'x'
				}
				return value, nil
			})
			reader := readerFunc(func(_ context.Context, h string) (*File, error) {
				reads++
				if h != checksum([]byte("abc")) {
					t.Fatal("nonmanifest blob selected")
				}
				return &File{ReadCloser: file, Size: 3, SHA256: h}, nil
			})
			id, kind := "01", "input"
			if fail == "unknown-test" {
				id = "02"
			}
			if fail == "path" {
				id = "../01"
			}
			if fail == "kind" {
				kind = "source"
			}
			got, err := New(store, reader).Download(context.Background(), node, "lease", id, kind)
			if fail == "none" {
				if err != nil || calls != 2 || reads != 1 {
					t.Fatalf("valid download: %v calls=%d reads=%d", err, calls, reads)
				}
				got.Close()
			} else if err == nil || got != nil {
				t.Fatal("unauthorized/corrupt material released")
			}
			if reads > 0 && !file.closed {
				t.Fatal("snapshot leaked")
			}
			if fail == "first-auth" && (!errors.Is(err, submission.ErrNode) || reads != 0) {
				t.Fatal("read before authorization")
			}
		})
	}
}

func TestManifestValidation(t *testing.T) {
	for _, change := range []func(*submission.TaskMaterials){
		func(v *submission.TaskMaterials) { v.Version = "different" },
		func(v *submission.TaskMaterials) { v.Language = "other" },
		func(v *submission.TaskMaterials) {
			v.Manifest = []byte(`{"version":"v1","language":"gnu++17","tests":[]}`)
		},
		func(v *submission.TaskMaterials) {
			v.Manifest = []byte(strings.ReplaceAll(string(v.Manifest), `"01"`, `"../01"`))
		},
		func(v *submission.TaskMaterials) {
			v.Manifest = []byte(strings.ReplaceAll(string(v.Manifest), checksum([]byte("abc")), "bad"))
		},
	} {
		value := testMaterials()
		change(&value)
		value.ManifestSHA256 = checksum(value.Manifest)
		store := materialStoreFunc(func(context.Context, submission.NodeIdentity, string) (submission.TaskMaterials, error) {
			return value, nil
		})
		if _, err := New(store, nil).Describe(context.Background(), submission.NodeIdentity{}, ""); !errors.Is(err, ErrUnavailable) {
			t.Fatal("invalid manifest accepted", err)
		}
	}
}
