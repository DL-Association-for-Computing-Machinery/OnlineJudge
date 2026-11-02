package sandbox_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

type compilerSnapshot struct{ vmPID, state string }

func parseCompilerPause(out string, commandErr error) (string, bool, error) {
	// Docker itself can return the same exit code as a remote shell. Absence
	// requires a successful command and its explicit, fixed acknowledgement.
	if commandErr != nil {
		return "", false, errors.New("compiler pause command outcome unavailable")
	}
	value := strings.TrimSpace(out)
	if value == "no-compiler" {
		return "", true, nil
	}
	pid, err := strconv.Atoi(value)
	if err != nil || pid <= 1 || strconv.Itoa(pid) != value {
		return "", false, errors.New("invalid compiler pause acknowledgement")
	}
	return value, false, nil
}

// Docker top reports VM PIDs. This parser never treats them as container PIDs
// for signalling; the dedicated fixture pauses the unique compiler internally.
func parseCompilerTop(out string) (compilerSnapshot, error) {
	var result compilerSnapshot
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || strings.Join(strings.Fields(lines[0]), " ") != "PID COMMAND STAT" {
		return result, errors.New("invalid compiler process header")
	}
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) != 3 {
			return compilerSnapshot{}, errors.New("invalid compiler process evidence")
		}
		if f[1] != "cc1plus" {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil || pid <= 1 || strconv.Itoa(pid) != f[0] || result.vmPID != "" {
			return compilerSnapshot{}, errors.New("ambiguous compiler identity")
		}
		result = compilerSnapshot{vmPID: f[0], state: f[2]}
	}
	return result, nil
}

func TestCompilerProcessEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, text, pid, state string
		valid                  bool
	}{
		{"empty set", "PID COMMAND STAT\n42 go-judge Sl", "", "", true},
		{"running", "PID COMMAND STAT\n42 go-judge Sl\n55 cc1plus R", "55", "R", true},
		{"stopped", "PID COMMAND STAT\n55 cc1plus Tl", "55", "Tl", true},
		{"header", "PID COMMAND\n55 cc1plus", "", "", false},
		{"missing state", "PID COMMAND STAT\n55 cc1plus", "", "", false},
		{"init", "PID COMMAND STAT\n1 cc1plus T", "", "", false},
		{"noncanonical pid", "PID COMMAND STAT\n+55 cc1plus T", "", "", false},
		{"duplicate", "PID COMMAND STAT\n55 cc1plus T\n56 cc1plus T", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCompilerTop(tc.text)
			if (err == nil) != tc.valid || got.vmPID != tc.pid || got.state != tc.state {
				t.Fatalf("unexpected process evidence: %+v %v", got, err)
			}
		})
	}
}

func TestCompilerPauseEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, output, pid string
		commandErr        error
		absent, valid     bool
	}{
		{"absent acknowledgement", "no-compiler\n", "", nil, true, true},
		{"paused acknowledgement", "55\n", "55", nil, false, true},
		{"empty", "", "", nil, false, false},
		{"init", "1", "", nil, false, false},
		{"noncanonical", "+55", "", nil, false, false},
		{"multiple", "55\n56", "", nil, false, false},
		{"cli failure with sentinel", "no-compiler\n", "", errors.New("CLI failed"), false, false},
		{"cli failure after signal", "55\n", "", errors.New("CLI failed"), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pid, absent, err := parseCompilerPause(tc.output, tc.commandErr)
			if (err == nil) != tc.valid || pid != tc.pid || absent != tc.absent {
				t.Fatalf("unexpected pause evidence: %q %t %v", pid, absent, err)
			}
		})
	}
}
