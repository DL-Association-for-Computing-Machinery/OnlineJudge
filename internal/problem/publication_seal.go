package problem

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

// Seal verifies the selected formal bytes and persists the display artifact.
// The caller holds the shared material operation slot, supplies a deadline,
// and must recheck authorization and exact evidence before publishing any reference.
// A failed or interrupted call may leave an unreferenced immutable display object.
func (p PublicationMaterialPlan) Seal(ctx context.Context, store materialstore.Store) (PublicationMaterials, error) {
	if ctx == nil || store == nil || p.problemID == "" || len(p.statement.files) == 0 {
		return PublicationMaterials{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return PublicationMaterials{}, err
	}
	if err := verifyPublicationFormalMaterials(ctx, store, p.formal); err != nil {
		return PublicationMaterials{}, err
	}
	result, raw, err := p.statementBytes(ctx, store)
	if err != nil {
		return PublicationMaterials{}, err
	}
	o, err := store.Put(ctx, result.Statement, bytes.NewReader(raw))
	if err != nil {
		if errors.Is(err, materialstore.ErrCommitUnknown) {
			return PublicationMaterials{}, materialstore.ErrCommitUnknown
		}
		return PublicationMaterials{}, referenceObjectError(ctx, err)
	}
	if o != result.Statement {
		return PublicationMaterials{}, ErrStore
	}
	if err := ctx.Err(); err != nil {
		return PublicationMaterials{}, err
	}
	return result, nil
}

func verifyPublicationFormalMaterials(ctx context.Context, store ReferenceObjectReader, plan ReferencePlan) error {
	open := persistedReferenceOpen(store)
	snapshot, err := plan.FreezeMaterials(ctx, func(ctx context.Context, obj materialstore.Object) (io.ReadCloser, error) {
		return open(ctx, fileReference{SHA256: obj.SHA256, Size: obj.Size})
	})
	if err != nil {
		return err
	}
	if err := snapshot.Close(); err != nil {
		return ErrStore
	}
	return ctx.Err()
}
