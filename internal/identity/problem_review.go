package identity

import "errors"

// ErrReviewerRequired means the subject has no current explicit review grant.
var ErrReviewerRequired = errors.New("current problem reviewer permission required")

// ProblemReviewerRole is point-in-time maintenance metadata. It never conveys
// access to a review item; an explicit assignment is also required.
type ProblemReviewerRole struct {
	SubjectID string `json:"subject_id"`
	Enabled   bool   `json:"enabled"`
	Revision  int64  `json:"revision"`
}
