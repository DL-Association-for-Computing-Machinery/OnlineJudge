package judging_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
)

func TestLibraryCheckerSemantics(t *testing.T) {
	limits := judging.TextLimits{Bytes: 2 << 20, TokenBytes: 1 << 16}
	for _, tc := range []struct {
		name, reference, output string
		want                    bool
	}{
		{"four separators", "YES 1 2", " \tYES\r1\n2\r\n", true},
		{"vertical tab", "1 2", "1\v2", false},
		{"form feed", "1 2", "1\f2", false},
		{"literal vertical tab", "1\v2", "1\v2", true},
		{"case", "YES", "yes", false},
		{"integer spelling", "1", "01", false},
		{"float spelling", "1", "1.0", false},
		{"unicode", "中文", "中文\n", true},
		{"unicode space", "1 2", "1\u00a02", false},
		{"missing", "1 2", "1", false},
		{"extra", "1", "1 2", false},
		{"empty", "", " \t\r\n", true},
		{"empty vs vertical tab", "", "\v", false},
		{"invalid encoding", "1", string([]byte{0xff}), false},
		{"over M0 budget", strings.Repeat("1 ", (1<<19)+1), strings.Repeat("1\n", (1<<19)+1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := judging.CompareLibraryCheckerText([]byte(tc.reference), []byte(tc.output), limits)
			if err != nil || got != tc.want {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	// Both independently versioned contracts must coexist.
	if ok, err := judging.CompareText([]byte("1 2"), []byte("1\v2"), true); err != nil || !ok {
		t.Fatalf("M0 whitespace changed: %v, %v", ok, err)
	}
}

func TestLibraryCheckerLimitsAndReferencePrecedence(t *testing.T) {
	for _, limits := range []judging.TextLimits{{}, {Bytes: -1, TokenBytes: 1}, {Bytes: judging.MaxTextBytes + 1, TokenBytes: 1}, {Bytes: 4, TokenBytes: 0}, {Bytes: 4, TokenBytes: 5}, {Bytes: judging.MaxTextBytes, TokenBytes: (1 << 16) + 1}} {
		if ok, err := judging.CompareLibraryCheckerText(nil, nil, limits); ok || !errors.Is(err, judging.ErrTextLimits) {
			t.Fatalf("limits %+v: %v %v", limits, ok, err)
		}
	}
	limits := judging.TextLimits{Bytes: 8, TokenBytes: 4}
	for _, tc := range []struct {
		reference, output string
		want              error
	}{
		{"1234 5678", "x", judging.ErrReference},
		{"1 12345", "x", judging.ErrReference},
		{"1 12345", "123456789", judging.ErrReference},
		{string([]byte{0xff}), "x", judging.ErrReference},
		{"1", "123456789", judging.ErrOutput},
		{"1", "x 12345", judging.ErrOutput},
	} {
		if ok, err := judging.CompareLibraryCheckerText([]byte(tc.reference), []byte(tc.output), limits); ok || !errors.Is(err, tc.want) {
			t.Fatalf("%q vs %q: %v %v; want %v", tc.reference, tc.output, ok, err, tc.want)
		}
	}
	if ok, err := judging.CompareLibraryCheckerText([]byte("1234 567"), []byte("1234\n567"), limits); !ok || err != nil {
		t.Fatalf("inclusive byte/token boundary rejected: %v %v", ok, err)
	}
	if ok, err := judging.CompareLibraryCheckerText(nil, nil, judging.TextLimits{Bytes: judging.MaxTextBytes, TokenBytes: 1 << 16}); !ok || err != nil {
		t.Fatalf("maximum valid configuration rejected: %v %v", ok, err)
	}
	badReference := []byte("1 " + strings.Repeat("a", (1<<16)+1))
	largeOutput := []byte(strings.Repeat("1 ", (1<<19)+1))
	if _, err := judging.CompareText(badReference, largeOutput, true); !errors.Is(err, judging.ErrOutput) {
		t.Fatalf("M0 error precedence changed: %v", err)
	}
	if _, err := judging.CompareLibraryCheckerText(badReference, largeOutput, judging.TextLimits{Bytes: judging.TextBudget, TokenBytes: 1 << 16}); !errors.Is(err, judging.ErrReference) {
		t.Fatalf("LC complete reference precedence lost: %v", err)
	}
	if ok, err := judging.CompareLibraryCheckerText([]byte("1"), []byte("\xff12345"), limits); ok || err != nil {
		t.Fatalf("invalid output encoding must precede output token checks: %v %v", ok, err)
	}
}
