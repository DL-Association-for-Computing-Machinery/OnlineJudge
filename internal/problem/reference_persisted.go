package problem

import (
	"context"
	"errors"
	"io"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

// ReferenceObjectReader opens private objects by their immutable identity.
type ReferenceObjectReader interface {
	Open(context.Context, materialstore.Object) (*materialstore.Snapshot, error)
}

// FreezeReference binds detached, previously authorized draft metadata to the
// trusted import before reading any object. The caller must Close the returned
// snapshot and recheck live authority before using its result. No program runs.
func (d Draft) FreezeReference(ctx context.Context, id, owner string, expected ImportIdentity, store ReferenceObjectReader) (*ReferenceSnapshot, error) {
	if ctx == nil || store == nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := d.BindReferenceContent(id, owner, expected)
	if err != nil {
		return nil, err
	}
	var admission diskReferenceAdmission
	if c.SourceID == "library-checker:aplusb" {
		admission = diskReferenceAdmission{path: "sample/aplusb", limits: SmallReferenceLimits()}
	} else {
		admission, err = referenceDiskAdmission(c.SourceID)
		if err != nil {
			return nil, err
		}
	}
	// The complete import revision already binds the document to the exact
	// logical inventory. Only objects selected by the admitted layout are
	// opened; bundle paths are never interpreted by the persistent reader.
	open := func(ctx context.Context, ref fileReference) (io.ReadCloser, error) {
		o := ref.object()
		snapshot, err := store.Open(ctx, o)
		if err != nil {
			return nil, referenceObjectError(ctx, err)
		}
		if snapshot == nil {
			return nil, materialstore.ErrUnavailable
		}
		if snapshot.Object != o {
			if err := snapshot.Close(); err != nil {
				return nil, errors.Join(materialstore.ErrIntegrity, materialstore.ErrUnavailable)
			}
			return nil, materialstore.ErrIntegrity
		}
		return snapshot, nil
	}
	return freezeReferenceSnapshot(ctx, c, c.SourceID, admission, open, nil)
}

func referenceObjectError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Keep retryable classifications without returning storage paths or raw I/O errors.
	for _, known := range []error{materialstore.ErrIntegrity, materialstore.ErrNotFound, materialstore.ErrBusy, materialstore.ErrClosed, materialstore.ErrInvalid} {
		if errors.Is(err, known) {
			return known
		}
	}
	return materialstore.ErrUnavailable
}
