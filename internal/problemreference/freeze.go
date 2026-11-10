package problemreference

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

// FreezeDraft prepares the complete material plan from a previously authorized
// draft and queued receipt. It requires the originally frozen profile to still
// match the supported profile byte for byte. It performs I/O outside any
// database transaction; the caller must recheck live state and authority and
// Close the snapshot. It does not advance the request or admit execution.
func (r Request) FreezeDraft(ctx context.Context, draft problem.Draft, store problem.ReferenceObjectReader) (*problem.ReferenceSnapshot, error) {
	if ctx == nil || store == nil || !r.bindable() {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	profile, _, err := problemvalidation.ResolveSource(r.Specification.Identity.SourceID)
	if err != nil {
		return nil, ErrSourceChanged
	}
	raw, err := json.Marshal(profile)
	if err != nil || !bytes.Equal(raw, r.Specification.ProfileJSON) {
		return nil, ErrSourceChanged
	}
	return draft.FreezeReference(ctx, r.DraftID, r.Owner, r.Specification.Identity, store)
}
