package workerhost

import "context"

// ReferenceSlot identifies the single separately provisioned reference sandbox.
// M0 parsing never selects this container or its canonical ownership lock.
type ReferenceSlot struct{}

// ContainerName is fixed; reference execution accepts no Docker alias override.
func (ReferenceSlot) ContainerName() string { return "oj-reference-sandbox" }

// LockName is shared across checkouts and distinct from both M0 locks.
func (ReferenceSlot) LockName() string { return "slot-reference.lock" }

// OpenReference checks an already running fixed reference slot. The caller must
// hold its clean canonical lock for the complete lifetime and independently Stop.
func OpenReference(ctx context.Context) (*Host, error) {
	return openConfiguredSlot(ctx, runCommand, ReferenceSlot{}.ContainerName(), true)
}

// StartReference starts only the pre-existing stopped owned reference slot.
// Dirty marking, non-nil Host on ambiguous start, and Stop follow StartM0Slot.
func StartReference(ctx context.Context) (*Host, error) {
	return startConfiguredSlot(ctx, runCommand, ReferenceSlot{}.ContainerName(), true)
}
