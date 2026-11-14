package problem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

func (m PublicationMaterials) valid() bool {
	return identity.ValidID(m.ProblemID) && validHash(m.Version) && validHash(m.FormalPlan) &&
		m.Import.SourceID != "" && len(m.Import.SourceID) <= 128 && validHash(m.Import.Version) &&
		validHash(m.Import.ImportRevision) && validHash(m.Import.SourceLockSHA256) &&
		validHash(m.Statement.SHA256) && m.Statement.Size > 0 && m.Statement.Size <= MaxPublicationStatementBytes
}

// ReadSealedPublicationStatement reads an independently trusted display
// reference. It neither establishes authorization nor checks private objects.
// Callers must hold the shared material budget and supply a bounded context.
func ReadSealedPublicationStatement(ctx context.Context, store ReferenceObjectReader, expected PublicationMaterials) (PublicationStatement, error) {
	var zero PublicationStatement
	if ctx == nil || store == nil || !expected.valid() {
		return zero, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	raw, err := readPreviewBytes(ctx, store, expected.Statement)
	if ctx.Err() != nil {
		return zero, ctx.Err()
	}
	if err != nil || hash(raw) != expected.Statement.SHA256 || !utf8.Valid(raw) {
		return zero, ErrStore
	}
	var display PublicationStatement
	if json.Unmarshal(raw, &display) != nil || display.ProblemID != expected.ProblemID || display.Version != expected.Version ||
		!validPublicationProjection(display) {
		return zero, ErrStore
	}
	canonical, err := json.Marshal(display)
	if err != nil || !bytes.Equal(raw, canonical) {
		return zero, ErrStore
	}
	version, err := publicationVersion(display, expected.FormalPlan)
	if err != nil || version != expected.Version {
		return zero, ErrStore
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return display, nil
}

func validPublicationProjection(display PublicationStatement) bool {
	if display.Title == "" || len(display.Title) > 512 || display.Language != "en" ||
		display.Examples == nil || len(display.Examples) > 1024 || len(display.Images) > MaxPreviewImages {
		return false
	}
	total := int64(len(display.Markdown))
	last := ""
	for _, example := range display.Examples {
		n, err := strconv.Atoi(strings.TrimPrefix(example.ID, "case-"))
		if err != nil || n < 1 || n > 1024 ||
			example.ID != fmt.Sprintf("case-%04d", n) || example.ID <= last {
			return false
		}
		last = example.ID
		total += int64(len(example.Input)) + int64(len(example.Output))
	}
	last = ""
	var pixels int64
	for _, image := range display.Images {
		if !validImageSource(image.Source) || image.Source <= last || image.MediaType != "image/png" ||
			len(image.Data) == 0 || int64(len(image.Data)) > MaxPreviewImageBytes {
			return false
		}
		config, err := png.DecodeConfig(bytes.NewReader(image.Data))
		if err != nil || config.Width != image.Width || config.Height != image.Height ||
			image.Width < 1 || image.Height < 1 || image.Width > 4096 || image.Height > 4096 {
			return false
		}
		area := int64(image.Width) * int64(image.Height)
		if area > maxImagePixels || area > maxPreviewPixels-pixels {
			return false
		}
		last = image.Source
		pixels += area
		total += int64(len(image.Data))
	}
	return total <= MaxPreviewBytes
}

// VerifySealedPublicationMaterials reopens both sets of material for an
// independently trusted binding. Success is a point-in-time observation,
// not authorization to publish or permission to expose private plan objects.
// Callers hold the shared material budget and supply a bounded context.
func VerifySealedPublicationMaterials(ctx context.Context, store ReferenceObjectReader, expected PublicationMaterials, plan ReferencePlan) error {
	if ctx == nil || store == nil || !expected.valid() {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Freeze the metadata before opening the display; its storage adapter
	// must not be able to change the private references we just authenticated.
	plan.Cases = slices.Clone(plan.Cases)
	plan.Dependencies = slices.Clone(plan.Dependencies)
	bound := ImportIdentity{SourceID: plan.SourceID, SourceLockSHA256: plan.SourceLockSHA256,
		Version: plan.Version, ImportRevision: plan.ImportRevision}
	if bound != expected.Import || plan.ValidateMaterials() != nil || plan.Digest() != expected.FormalPlan {
		return ErrInvalid
	}
	if _, err := ReadSealedPublicationStatement(ctx, store, expected); err != nil {
		return err
	}
	return verifyPublicationFormalMaterials(ctx, store, plan)
}
