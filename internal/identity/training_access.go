package identity

import "errors"

// ErrTrainingAccountRequired means the current subject is not a verified personal account.
var ErrTrainingAccountRequired = errors.New("verified personal account required for training")

// TrainingActor is a server-owned observation from an ordinary session read.
// It is neither a credential nor permission to access a particular resource.
type TrainingActor struct {
	SubjectID string
	SessionVersion
}

// Valid checks shape only; the business transaction must recheck authority.
func (a TrainingActor) Valid() bool { return ValidID(a.SubjectID) && a.SessionVersion.Valid() }
