package judging

import "errors"

// CheckerProtocol selects a fixed result contract. A program cannot negotiate
// its protocol through its exit status or diagnostics.
type CheckerProtocol uint8

const (
	CheckerICPC CheckerProtocol = iota + 1
	CheckerTestlib0945
)

// CheckerOutcome is a point decision, never a submission verdict.
type CheckerOutcome uint8

const (
	CheckerSystemError CheckerOutcome = iota
	CheckerAccept
	CheckerWrongAnswer
)

var ErrCheckerResult = errors.New("invalid checker result")

// DecodeCheckerExit takes an actual normal process exit. A sandbox fault
// overrides the exit code. The Testlib branch is for the pinned build only:
// it uses disjoint codes and normalizes all partial results before exit.
func DecodeCheckerExit(protocol CheckerProtocol, exited bool, code int) (CheckerOutcome, error) {
	if !exited {
		return CheckerSystemError, ErrCheckerResult
	}
	switch protocol {
	case CheckerICPC:
		switch code {
		case 42:
			return CheckerAccept, nil
		case 43:
			return CheckerWrongAnswer, nil
		}
	case CheckerTestlib0945:
		switch code {
		case 50:
			return CheckerAccept, nil
		case 51, 52, 53, 54: // wrong answer, malformed, extra output, contestant EOF
			return CheckerWrongAnswer, nil
		}
	}
	return CheckerSystemError, ErrCheckerResult
}
