package submission

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNode           = errors.New("node disabled or credential version invalid")
	ErrNoTask         = errors.New("no task available within node capacity")
	ErrLease          = errors.New("lease expired or superseded")
	ErrResultConflict = errors.New("attempt already reported different result")
)

// NodeIdentity is supplied only by the authenticated transport, never request JSON.
type NodeIdentity struct {
	ID                string
	CredentialVersion int64
}

// Lease pins independent generations and an attempt; Token is never a public receipt.
type Lease struct {
	Token, SubmissionID                 string
	JudgeGeneration, RecoveryGeneration int64
	AttemptNumber                       int32
	Until                               time.Time
}

// Result contains a verdict and bounded evidence digest. Execution validation belongs to P2.
// Field names preserve existing persisted result fingerprints.
type Result struct {
	Verdict        string
	EvidenceSHA256 string
}

// LeaseStore authorizes the current node and lease atomically with each operation.
type LeaseStore interface {
	Claim(context.Context, NodeIdentity) (Lease, error)
	Renew(context.Context, NodeIdentity, string) (time.Time, error)
	Report(context.Context, NodeIdentity, string, Result) error
}
