package postgres

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/jackc/pgx/v5"
)

var errReferenceReservationBusy = fmt.Errorf("%w: active reservation", problemreference.ErrClaimUnavailable)

// ReferenceClaimer binds a single trusted node and source lock to the ledger.
type ReferenceClaimer struct {
	store         *Store
	lockNode      func(context.Context, pgx.Tx, submission.NodeIdentity) error
	nodeID        string
	sourceLock    string
	packageSource string
}

// NewReferenceClaimer uses explicit trusted configuration, not Worker input.
func NewReferenceClaimer(s *Store, lockNode func(context.Context, pgx.Tx, submission.NodeIdentity) error, nodeID, sourceLock string) (*ReferenceClaimer, error) {
	if s == nil || lockNode == nil || !problemreference.ValidReferenceNodeID(nodeID) || !problem.ValidImportRevision(sourceLock) {
		return nil, problemreference.ErrInvalid
	}
	return &ReferenceClaimer{store: s, lockNode: lockNode, nodeID: nodeID, sourceLock: sourceLock}, nil
}

// WithLocalPackage restricts this dedicated local node to one explicit source.
func (c *ReferenceClaimer) WithLocalPackage(source string) *ReferenceClaimer {
	next := *c
	next.packageSource = source
	return &next
}

func (c *ReferenceClaimer) resolveProfile(source string) (problemvalidation.Profile, error) {
	if c.packageSource != "" && source != c.packageSource {
		return problemvalidation.Profile{}, problemreference.ErrClaimUnavailable
	}
	p, _, err := problemvalidation.ResolveReferenceProfile(source, c.packageSource != "")
	return p, err
}

// ClaimReference locks node, author, draft, request, plan and reservation in
// that order. It performs no object I/O and never changes the queued request.
func (c *ReferenceClaimer) ClaimReference(ctx context.Context, node submission.NodeIdentity, input problemreference.ClaimInput) (problemreference.WorkerClaim, error) {
	return c.claimReference(ctx, node, input, false)
}

func (c *ReferenceClaimer) claimReference(ctx context.Context, node submission.NodeIdentity, input problemreference.ClaimInput, firstOnly bool) (problemreference.WorkerClaim, error) {
	var zero problemreference.WorkerClaim
	if c == nil || ctx == nil || !input.Valid() {
		return zero, problemreference.ErrInvalid
	}
	if node.ID != c.nodeID || node.CredentialVersion < 1 {
		return zero, submission.ErrNode
	}
	profile, err := c.resolveProfile(input.SourceID)
	if err != nil {
		return zero, problemreference.ErrClaimUnavailable
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return zero, storageError{err}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := c.store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer rollback(tx)
	if err := c.lockNode(ctx, tx, node); err != nil {
		return zero, err
	}
	q := referencedb.New(tx)
	request, err := c.lockClaimRequest(ctx, tx, q, input, raw)
	if err != nil {
		return zero, err
	}
	if firstOnly {
		already, err := q.HasReferenceDispatch(ctx, request.ID)
		if err != nil {
			return zero, storageError{err}
		}
		if already {
			return zero, problemreference.ErrClaimUnavailable
		}
	}
	row, err := q.ReferenceExpectedPlanForShare(ctx, request.ID)
	if err != nil {
		return zero, referenceClaimError(err)
	}
	plan := problemreference.ExpectedPlan{Encoding: row.PlanEncoding, SHA256: row.PlanSha256}
	if plan.SHA256 != input.PlanSHA256 || !plan.ValidFor(request.Specification) {
		return zero, problemreference.ErrClaimUnavailable
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return zero, storageError{err}
	}
	token := hex.EncodeToString(nonce[:])
	until, err := q.ReserveReferenceWorkerClaim(ctx, referencedb.ReserveReferenceWorkerClaimParams{
		RequestID: request.ID, NodeID: node.ID, CredentialVersion: node.CredentialVersion, LeaseToken: token, PlanSha256: plan.SHA256,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, errReferenceReservationBusy
	}
	if err != nil {
		return zero, referenceClaimError(err)
	}
	if !until.Valid || until.InfinityModifier != 0 {
		return zero, storageError{problemreference.ErrInvalid}
	}
	n, err := q.RecordReferenceDispatch(ctx, request.ID)
	if err != nil {
		return zero, storageError{err}
	}
	if firstOnly && n != 1 {
		return zero, problemreference.ErrClaimUnavailable
	}
	if err := commitTx(ctx, tx, true); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, errors.Join(problemreference.ErrCommitUnknown, err)
	}
	return problemreference.WorkerClaim{RequestID: request.ID, SourceID: input.SourceID,
		PlanSHA256: plan.SHA256, Token: token, Until: until.Time}, nil
}

func (c *ReferenceClaimer) lockClaimRequest(ctx context.Context, tx pgx.Tx, q *referencedb.Queries, input problemreference.ClaimInput, profile []byte) (problemreference.Request, error) {
	return c.lockReferenceRequest(ctx, tx, q, input, profile, false)
}

func (c *ReferenceClaimer) lockReferenceRequest(ctx context.Context, tx pgx.Tx, q *referencedb.Queries, input problemreference.ClaimInput, profile []byte, reporting bool) (problemreference.Request, error) {
	var zero problemreference.Request
	// Read only to discover the original actor before taking predecessor locks.
	row, err := q.ReferenceRequestByID(ctx, input.RequestID)
	if err != nil {
		return zero, referenceClaimError(err)
	}
	original, err := readRequest(row)
	if err != nil {
		return zero, err
	}
	if (original.State != problemreference.Queued && (!reporting || original.State != problemreference.Reported)) || original.Specification.Identity.SourceID != input.SourceID {
		return zero, problemreference.ErrClaimUnavailable
	}
	actor := original.Authorization.Actor
	authority, err := c.store.access.LockAuthorSnapshot(ctx, tx, actor)
	if err != nil {
		return zero, referenceClaimError(err)
	}
	if authority != original.Authorization {
		return zero, problemreference.ErrClaimUnavailable
	}
	draft, err := c.store.access.LockOwnDraftIdentity(ctx, tx, actor, original.DraftID)
	if err != nil {
		return zero, referenceClaimError(err)
	}
	// A first report releases queued capacity. Acquire policy BEFORE the
	// request row, in the same order as cancellation and admission.
	if reporting && original.State == problemreference.Queued {
		if _, err := lockPolicy(ctx, q); err != nil {
			return zero, err
		}
	}
	row, err = q.ReferenceRequestByIDForUpdate(ctx, original.ID)
	if err != nil {
		return zero, referenceClaimError(err)
	}
	current, err := readRequest(row)
	if err != nil {
		return zero, err
	}
	i := current.Specification.Identity
	if current.Authorization != authority ||
		current.ID != original.ID || current.Owner != actor.SubjectID || current.DraftID != original.DraftID ||
		i != original.Specification.Identity ||
		!bytes.Equal(current.Specification.ProfileJSON, original.Specification.ProfileJSON) ||
		draft.ID != current.DraftID || draft.Owner != actor.SubjectID || draft.SourceID != i.SourceID ||
		draft.Version != i.Version || draft.ImportRevision != i.ImportRevision {
		return zero, problemreference.ErrClaimUnavailable
	}
	if c.packageSource != "" && (i.SourceID != c.packageSource || i.SourceLockSHA256 != c.sourceLock || !bytes.Equal(current.Specification.ProfileJSON, profile)) {
		return zero, problemreference.ErrClaimUnavailable
	}
	if reporting && current.State == problemreference.Reported {
		return current, nil // Historical receipt, not a new execution/admission.
	}
	if current.State != problemreference.Queued || original.State != problemreference.Queued || i.SourceLockSHA256 != c.sourceLock || !bytes.Equal(current.Specification.ProfileJSON, profile) {
		return zero, problemreference.ErrClaimUnavailable
	}
	return current, nil
}

func referenceClaimError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, identity.ErrConflict) ||
		errors.Is(err, identity.ErrForbidden) || errors.Is(err, identity.ErrRecovering) ||
		errors.Is(err, problem.ErrNotFound) {
		return problemreference.ErrClaimUnavailable
	}
	return storageError{err}
}
