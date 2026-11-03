package experiment

// HostFailureStep carries bounded control-operation evidence across adapters.
// It contains no command, path, object identity, output or error text.
type HostFailureStep struct {
	Phase             string
	Completed         bool
	ElapsedMS         int64
	ElapsedCapped     bool
	ContextBefore     string
	HasDeadline       bool
	RemainingMS       int64
	RemainingCapped   bool
	CallAttempted     bool
	LocalProcessStart string
}

// HostFailureTrace is a value-only internal contract, not a report DTO or part
// of any execution profile. A projection must validate Count and all enums.
type HostFailureTrace struct {
	Phase         string
	LastCompleted string
	ElapsedMS     int64
	ElapsedCapped bool
	Count         int
	Steps         [5]HostFailureStep
}
