package identity

import "errors"

// ErrPlatformAdminRequired indicates no matching current platform admin grant.
var ErrPlatformAdminRequired = errors.New("current platform administrator permission required")

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
