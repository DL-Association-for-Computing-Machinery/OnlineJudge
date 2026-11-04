package identity

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrForbidden means current platform permission is absent or revoked.
var ErrForbidden = errors.New("current problem author permission required")

// ProblemActor is a server-owned session observation, never a credential or
// client-selected subject. A business transaction must lock and recheck it.
type ProblemActor struct {
	SubjectID string
	SessionVersion
}

// Valid checks shape only, not authentication or permission.
func (a ProblemActor) Valid() bool { return ValidID(a.SubjectID) && a.SessionVersion.Valid() }

// ProblemAuthorSnapshot records the identity generation and author revision
// checked under one caller-owned transaction. It is not a reusable permission,
// proof of commit, or a credential. Later operations must lock and check current
// authority again and compare the complete snapshot, including RoleRevision:
// revocation followed by regrant must not revive an older task's authority.
type ProblemAuthorSnapshot struct {
	Actor        ProblemActor
	RoleRevision int64
}

// ProblemAuthorRole is administrative metadata, not reusable authorization.
type ProblemAuthorRole struct {
	SubjectID string `json:"subject_id"`
	Enabled   bool   `json:"enabled"`
	Revision  int64  `json:"revision"`
}

// ProblemRoleChange is an explicit trusted-maintenance CAS operation. Operator
// is attribution supplied by that maintenance boundary, not a verified login.
type ProblemRoleChange struct {
	SubjectID        string
	ExpectedRevision int64
	Enabled          bool
	Operator         string
	Reason           string
}

// Valid rejects malformed audit attribution and absent CAS preconditions.
func (c ProblemRoleChange) Valid() bool {
	if !ValidID(c.SubjectID) || c.ExpectedRevision < 0 || !auditText(c.Operator, 80) || !auditText(c.Reason, 512) {
		return false
	}
	return true
}

func auditText(text string, max int) bool {
	if len(text) == 0 || len(text) > max || !utf8.ValidString(text) || strings.TrimSpace(text) != text {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
