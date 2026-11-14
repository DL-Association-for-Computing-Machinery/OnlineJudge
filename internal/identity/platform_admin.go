package identity

import "errors"

// ErrPlatformAdminRequired indicates no matching current platform admin grant.
var ErrPlatformAdminRequired = errors.New("current platform administrator permission required")

// PlatformAdminOperatorPrefix reserves HTTP-attributed audit operators.
const PlatformAdminOperatorPrefix = "platform-admin:"

// PlatformAdminRoleRequest contains only target and audit fields for a change.
// The operator is derived from a separately authenticated management session.
type PlatformAdminRoleRequest struct {
	SubjectID        string
	ExpectedRevision int64
	Enabled          bool
	Reason           string
}

// Valid checks the complete target, CAS and reason shape.
func (r PlatformAdminRoleRequest) Valid() bool {
	return ValidID(r.SubjectID) && r.ExpectedRevision >= 0 && r.ExpectedRevision < 9223372036854775807 && auditText(r.Reason, 512)
}

// PlatformAdminRole is maintenance metadata and confers no session rights.
type PlatformAdminRole struct {
	SubjectID string `json:"subject_id"`
	Revision  int64  `json:"revision"`
	Enabled   bool   `json:"enabled"`
}

// PlatformAdminChange is a trusted maintenance request, never browser input.
type PlatformAdminChange struct {
	SubjectID        string
	ExpectedRevision int64
	Enabled          bool
	Operator         string
	Reason           string
}

// Valid checks shape but does not authorize maintenance.
func (c PlatformAdminChange) Valid() bool {
	return ValidID(c.SubjectID) && c.ExpectedRevision >= 0 && ValidMaintenanceAttribution(c.Operator, c.Reason)
}

// PlatformAdminActor is an observation from a dedicated management session.
// The caller must authenticate that session separately before constructing one;
// neither this value nor a successful transaction can mint a session.
type PlatformAdminActor struct {
	SubjectID string
	SessionVersion
	RoleRevision int64
}

// Valid checks shape, not credential, scope, or permissions.
func (a PlatformAdminActor) Valid() bool {
	return ValidID(a.SubjectID) && a.SessionVersion.Valid() && a.RoleRevision > 0
}
