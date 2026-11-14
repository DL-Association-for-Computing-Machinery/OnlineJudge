package workerhost

import (
	"context"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// ReferenceSlot identifies the single separately provisioned reference sandbox.
// M0 parsing never selects this container or its canonical ownership lock.
type ReferenceSlot struct{}

// ContainerName is fixed; reference execution accepts no Docker alias override.
func (ReferenceSlot) ContainerName() string { return "oj-reference-sandbox" }

// LockName is shared across checkouts and distinct from both M0 locks.
func (ReferenceSlot) LockName() string { return referenceLock }

// OpenReference checks an already running fixed reference slot. The caller must
// hold its clean canonical lock for the complete lifetime and independently Stop.
func OpenReference(ctx context.Context) (*Host, error) {
	return openConfiguredSlot(ctx, runCommand, ReferenceSlot{}.ContainerName(), experiment.ReferenceSmall)
}

// StartReference starts only the pre-existing stopped owned reference slot.
// Dirty marking, non-nil Host on ambiguous start, and Stop follow StartM0Slot.
func StartReference(ctx context.Context) (*Host, error) {
	return startConfiguredSlot(ctx, runCommand, ReferenceSlot{}.ContainerName(), experiment.ReferenceSmall)
}

// ReferenceTierSlot is an opaque fixed slot. Its zero value is invalid, unlike
// the preserved legacy ReferenceSlot zero value, which means the small slot.
type ReferenceTierSlot struct {
	tier      experiment.ReferenceTier
	secondary bool
	java21    bool
}

// ReferenceSlotForTier rejects unknown tiers without returning a usable slot.
func ReferenceSlotForTier(tier experiment.ReferenceTier) (ReferenceTierSlot, error) {
	if _, err := experiment.ResolveReferenceTier(tier); err != nil {
		return ReferenceTierSlot{}, err
	}
	return ReferenceTierSlot{tier: tier}, nil
}

// UbuntuTextSlot selects a fixed slot on the dedicated Ubuntu experiment VM.
func UbuntuTextSlot(value string) (ReferenceTierSlot, error) {
	switch value {
	case "", "primary":
		return ReferenceTierSlot{tier: experiment.ReferenceAPlusBUbuntu}, nil
	case "secondary":
		return ReferenceTierSlot{tier: experiment.ReferenceAPlusBUbuntu, secondary: true}, nil
	default:
		return ReferenceTierSlot{}, errors.New("unsupported Ubuntu text slot")
	}
}

// WithJava21Image selects the pinned Java image without changing the slot identity.
func (s ReferenceTierSlot) WithJava21Image() ReferenceTierSlot {
	s.java21 = s.IsUbuntuAPlusB()
	return s
}

func (s ReferenceTierSlot) configuration() (name, lock string, tier experiment.ReferenceTier, err error) {
	switch s.tier {
	case experiment.ReferenceSmall:
		return ReferenceSlot{}.ContainerName(), ReferenceSlot{}.LockName(), s.tier, nil
	case experiment.ReferenceAPlusBUbuntu:
		if s.secondary {
			// Reuse a known ownership lock so even earlier binaries see quarantine.
			secondary, _ := ParseM0Slot("secondary")
			return "oj-aplusb-ubuntu-sandbox-secondary", secondary.LockName(), s.tier, nil
		}
		return "oj-aplusb-ubuntu-sandbox", ReferenceSlot{}.LockName(), s.tier, nil
	case experiment.ReferenceConvolution:
		return "oj-reference-convolution-sandbox", convolutionLock, s.tier, nil
	case experiment.ReferenceMedium, experiment.ReferenceMediumBulk, experiment.ReferencePrimality, experiment.ReferenceStaticRMQ, experiment.ReferenceStaticRangeSum, experiment.ReferenceAssociativeArray:
		return "oj-reference-medium-sandbox", mediumLock, s.tier, nil
	default:
		return "", "", 0, errors.New("unsupported reference slot")
	}
}

// ContainerName is empty for an invalid slot, never a default or arbitrary alias.
func (s ReferenceTierSlot) ContainerName() string { name, _, _, _ := s.configuration(); return name }

// LockName is empty for an invalid slot, never another slot's canonical lock.
func (s ReferenceTierSlot) LockName() string { _, name, _, _ := s.configuration(); return name }

// IsUbuntuAPlusB identifies the slot that uses the native pinned guest Engine.
func (s ReferenceTierSlot) IsUbuntuAPlusB() bool { return s.tier == experiment.ReferenceAPlusBUbuntu }

// OpenReferenceSlot verifies the separately provisioned fixed tier slot.
// Its caller holds the matching canonical lock until independent Stop completes.
func OpenReferenceSlot(ctx context.Context, slot ReferenceTierSlot) (*Host, error) {
	name, _, tier, err := slot.configuration()
	if err != nil {
		return nil, err
	}
	if slot.IsUbuntuAPlusB() {
		return nil, errors.New("the Ubuntu A+B experiment requires starting a stopped slot")
	}
	return openConfiguredSlot(ctx, runCommand, name, tier)
}

// StartReferenceSlot starts only a stopped owned fixed tier slot. Dirty marking
// and ambiguous-receipt cleanup follow StartM0Slot's unchanged requirements.
func StartReferenceSlot(ctx context.Context, slot ReferenceTierSlot) (*Host, error) {
	name, _, tier, err := slot.configuration()
	if err != nil {
		return nil, err
	}
	if slot.IsUbuntuAPlusB() {
		return startConfiguredHost(ctx, &Host{run: ubuntuCommand, tier: tier, java21: slot.java21}, name)
	}
	return startConfiguredSlot(ctx, runCommand, name, tier)
}
