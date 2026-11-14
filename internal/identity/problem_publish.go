package identity

import "errors"

// ErrPublisherRequired means the subject lacks an enabled publication grant.
var ErrPublisherRequired = errors.New("current problem publisher permission required")

// ProblemPublisherRole records the latest explicit maintenance grant. It
// conveys no review assignment or permission to expose a particular version.
type ProblemPublisherRole struct {
	SubjectID string `json:"subject_id"`
	Enabled   bool   `json:"enabled"`
	Revision  int64  `json:"revision"`
}
