// Package problemreference owns persistent reference-validation requests. A
// queued request has not verified materials or executed a program; the separate
// problemvalidation runner remains responsible for actual validation.
package problemreference

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

var (
	ErrInvalid       = errors.New("invalid reference request")
	ErrNotFound      = errors.New("reference request not found")
	ErrConflict      = errors.New("reference request key conflict")
	ErrSourceChanged = errors.New("reference request source is not configured at this revision")
	ErrPolicy        = errors.New("reference request admission is not configured")
	ErrFull          = errors.New("reference request queue is full")
	ErrStore         = errors.New("reference request storage unavailable")
	ErrCommitUnknown = errors.New("reference request commit outcome unknown; retain original key")
)

const (
	// IdentityProtocol fixes the meaning of the stored configuration bytes.
	IdentityProtocol = "oj.reference-request.v1"
	Queued           = "queued"
	Cancelled        = "cancelled"
)

// Request is an immutable intent and its current lifecycle receipt. The original
// authorization and configuration survive cancellation and later regrants.
// There is deliberately no Plan, attempt, verdict or claimed execution proof.
type Request struct {
	ID            string
	Owner         string
	RequestKey    string
	DraftID       string
	Specification Specification
	Authorization identity.ProblemAuthorSnapshot
	State         string
	CreatedAt     time.Time
	CancelledAt   *time.Time
}

// Limits bounds queued requests, not active sandbox slots or historical rows.
// Admission remains disabled until an administrator configures a policy.
type Limits struct {
	Global int64
	Owner  int64
}

// Valid keeps count queries and the configured queue strictly bounded.
func (l Limits) Valid() bool {
	return l.Global > 0 && l.Global <= 1024 && l.Owner > 0 && l.Owner <= l.Global
}

// Repository rechecks current author authority in the SAME short transaction as
// every operation. Catalog is trusted immutable server configuration, not input
// supplied by a browser or worker. Receipt and Cancel do not need a catalog.
type Repository interface {
	Enqueue(context.Context, identity.ProblemActor, string, string, *Catalog) (Request, error)
	Receipt(context.Context, identity.ProblemActor, string) (Request, error)
	Cancel(context.Context, identity.ProblemActor, string) (Request, error)
}
