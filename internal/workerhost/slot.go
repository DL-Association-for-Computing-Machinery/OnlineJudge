package workerhost

import "errors"

// M0Slot selects a pre-provisioned sandbox and its canonical ownership lock.
// The zero value preserves the original primary slot, including its lock file.
// Arbitrary container aliases and lock paths are deliberately unsupported.
type M0Slot struct{ secondary bool }

// ParseM0Slot accepts only the two fixed slots on the dedicated M0 VM.
func ParseM0Slot(value string) (M0Slot, error) {
	switch value {
	case "", "primary":
		return M0Slot{}, nil
	case "secondary":
		return M0Slot{secondary: true}, nil
	default:
		return M0Slot{}, errors.New("unsupported M0 slot")
	}
}

// ContainerName returns the fixed container name used only for initial lookup.
func (s M0Slot) ContainerName() string {
	if s.secondary {
		return "oj-m0-sandbox-secondary"
	}
	return container
}

// LockName returns the stable lock filename shared by all command checkouts.
func (s M0Slot) LockName() string {
	if s.secondary {
		return "slot-secondary.lock"
	}
	return "slot.lock"
}
