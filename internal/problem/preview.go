package problem

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// MaxPreviewBytes bounds text plus raw images, and separately text plus encoded
// PNGs. Repeated references count repeatedly; JSON text can expand sixfold.
const MaxPreviewBytes int64 = 1 << 20

// Preview contains the owner's statement, designated samples and static images.
// Markdown is data, never trusted HTML. No formal tests or programs are included.
type Preview struct {
	DraftID  string         `json:"draft_id"`
	Version  string         `json:"version"`
	Title    string         `json:"title"`
	Language string         `json:"language"`
	Markdown string         `json:"markdown"`
	Examples []Example      `json:"examples"`
	Images   []PreviewImage `json:"images,omitempty"`
}

// Example preserves exact valid UTF-8 bytes, including whitespace and newlines.
type Example struct {
	ID     string `json:"id"`
	Input  string `json:"input"`
	Output string `json:"output"`
}

type previewFile struct {
	name   string
	object materialstore.Object
}

func (s *Service) preview(ctx context.Context, credential redisstore.Credential, id string, acquire func() error) (Preview, error) {
	actor, err := s.actor(ctx, credential)
	if err != nil {
		return Preview{}, err
	}
	draft, err := s.repository.GetOwn(ctx, actor, id)
	if err != nil {
		return Preview{}, err
	}
	if err = acquire(); err != nil {
		return Preview{}, err
	}
	doc, files, err := previewFiles(draft, actor, id)
	if err != nil {
		return Preview{}, err
	}
	textCount := 1 + 2*len(doc.Examples)
	text := make([]string, textCount)
	for n, file := range files[:textCount] {
		text[n], err = s.previewText(ctx, file.object)
		if err != nil {
			return Preview{}, err
		}
	}
	images := make([]PreviewImage, 0, len(doc.Statement.Images))
	var pixels int64
	var outputBytes int64
	for _, value := range text {
		outputBytes += int64(len(value))
	}
	for _, file := range files[textCount:] {
		image, err := s.previewImage(ctx, file, &pixels)
		if err != nil || int64(len(image.Data)) > MaxPreviewBytes-outputBytes {
			return Preview{}, errors.Join(ErrStore, err)
		}
		outputBytes += int64(len(image.Data))
		image.Source = strings.TrimPrefix(file.name, "original/source/"+doc.Provenance.SourcePath+"/")
		images = append(images, image)
	}
	// Material verification is outside PG locks. Observe the actual credential
	// again and recheck owner/role after I/O before publishing any private bytes.
	latest, err := s.actor(ctx, credential)
	if err != nil {
		return Preview{}, err
	}
	if latest != actor {
		return Preview{}, identity.ErrConflict
	}
	current, err := s.repository.GetOwn(ctx, latest, id)
	if err != nil {
		return Preview{}, err
	}
	if current.ID != draft.ID || current.Owner != draft.Owner || current.SourceID != draft.SourceID || current.Version != draft.Version ||
		current.State != draft.State || current.ValidationState != draft.ValidationState ||
		!bytes.Equal(current.Document, draft.Document) || !reflect.DeepEqual(current.References, draft.References) {
		return Preview{}, ErrStore
	}
	if err = ctx.Err(); err != nil {
		return Preview{}, err
	}
	out := Preview{DraftID: id, Version: draft.Version, Title: doc.Title, Language: doc.Statement.Language,
		Markdown: text[0], Examples: make([]Example, len(doc.Examples)), Images: images}
	for n, exampleID := range doc.Examples {
		out.Examples[n] = Example{ID: exampleID, Input: text[1+2*n], Output: text[2+2*n]}
	}
	return out, nil
}

func previewFiles(draft Draft, actor identity.ProblemActor, id string) (document, []previewFile, error) {
	doc, err := parseDocument(draft.Document, draft.SourceID)
	if err != nil || draft.ID != id || draft.Owner != actor.SubjectID || draft.Version != doc.Version ||
		draft.State != "private-draft" || draft.ValidationState != "not-run" || len(draft.References) > 8192 {
		return document{}, nil, ErrStore
	}
	refs := make(map[string]materialstore.Object, len(draft.References))
	for _, ref := range draft.References {
		if _, exists := refs[ref.Name]; exists {
			return document{}, nil, ErrStore
		}
		refs[ref.Name] = ref.Object
	}
	if refs["draft.json"] != (materialstore.Object{SHA256: hash(draft.Document), Size: int64(len(draft.Document))}) {
		return document{}, nil, ErrStore
	}
	files := []previewFile{{"statement", doc.Statement.object()}}
	for _, c := range doc.TestCases {
		if c.Sample {
			files = append(files, previewFile{"tests/" + c.ID + "/input", c.Input.object()}, previewFile{"tests/" + c.ID + "/answer", c.Answer.object()})
		}
	}
	sources := make([]string, 0, len(doc.Statement.Images))
	for source := range doc.Statement.Images {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, source := range sources {
		files = append(files, previewFile{"original/source/" + doc.Provenance.SourcePath + "/" + source, doc.Statement.Images[source].object()})
	}
	var total int64
	for _, file := range files {
		if refs[file.name] != file.object || file.object.Size < 0 || file.object.Size > MaxPreviewBytes-total {
			return document{}, nil, ErrStore
		}
		total += file.object.Size
	}
	return doc, files, nil
}

func (s *Service) previewText(ctx context.Context, object materialstore.Object) (string, error) {
	raw, err := s.previewBytes(ctx, object)
	if err != nil || !utf8.Valid(raw) {
		return "", errors.Join(ErrStore, err)
	}
	return string(raw), nil
}

func (s *Service) previewBytes(ctx context.Context, object materialstore.Object) ([]byte, error) {
	snapshot, err := s.materials.Open(ctx, object)
	if err != nil {
		return nil, errors.Join(ErrStore, err)
	}
	if snapshot == nil {
		return nil, ErrStore
	}
	// Close each verified snapshot before opening the next: Local retains its
	// slot until Close, including with a one-operation deployment budget.
	raw, err := io.ReadAll(io.LimitReader(snapshot, object.Size+1))
	closeErr := snapshot.Close()
	if err != nil || closeErr != nil || snapshot.Object != object || int64(len(raw)) != object.Size {
		return nil, errors.Join(ErrStore, err, closeErr)
	}
	return raw, nil
}
