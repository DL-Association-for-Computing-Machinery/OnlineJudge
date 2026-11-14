package judging

import (
	"errors"
	"strings"
	"testing"
)

func TestDecimalTolerance(t *testing.T) {
	absolute := DecimalPolicy{Absolute: "0.1"}
	relative := DecimalPolicy{Relative: "0.01"}
	both := DecimalPolicy{Absolute: "0.1", Relative: "0.01", Text: []string{"RANGE"}}
	for _, tt := range []struct {
		name, ref, out string
		p              DecimalPolicy
		want           bool
	}{
		{"exact boundary", "-0.3", "-.4", absolute, true},
		{"inside", "-.3", "-.399999999999999999999", absolute, true},
		{"outside", "-.3", "-.400000000000000000001", absolute, false},
		{"relative boundary", "-10", "-1.01e1", relative, true},
		{"relative uses reference", "10", "10.1001", relative, false},
		{"absolute only qualifies", "0", "1e-1", both, true},
		{"relative only qualifies", "range 30", "RANGE 30.3", DecimalPolicy{Absolute: "0.1", Relative: "0.01", Text: []string{"range"}}, true},
		{"neither qualifies", "RANGE\v30", "range 30.31", both, false},
		{"relative at zero", "-0", "1e-400", relative, false},
		{"exact explicit zero", "1e501", "10e500", DecimalPolicy{Absolute: "0"}, true},
		{"bounds exceed binary doubles", "1e500", "1.001e500", relative, true},
		{"NaN output", "1", "NaN", absolute, false},
		{"infinite output", "1", "+Inf", absolute, false},
		{"fraction syntax rejected", "1", "1/1", absolute, false},
		{"hex syntax rejected", "1", "0x1p0", absolute, false},
		{"exponent limit", "0", "0e9999999", absolute, false},
		{"numbers over token limit", "1", strings.Repeat("0", 128) + "1", absolute, false},
		{"per-problem sensitive text", "RANGE 30", "range 30", DecimalPolicy{Absolute: "0", CaseSensitive: true, Text: []string{"RANGE"}}, false},
		{"extra token", "1", "1 0", absolute, false},
		{"missing token", "1 0", "1", absolute, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareDecimal([]byte(tt.ref), []byte(tt.out), tt.p)
			if err != nil || got != tt.want {
				t.Fatalf("got %t, %v; want %t", got, err, tt.want)
			}
		})
	}
}

func TestDecimalInvalidReferenceAndPolicy(t *testing.T) {
	p := DecimalPolicy{Absolute: "0.01", Text: []string{"NaN", "RANGE"}}
	if ok, err := CompareDecimal([]byte("NaN RANGE 1"), []byte("nan range 1"), p); err != nil || !ok {
		t.Fatalf("explicit text lost its meaning: %t, %v", ok, err)
	}
	for _, ref := range []string{"", "1 4e+", "1 -Inf", "1 1e-513", "1 BAD", strings.Repeat("1 ", 4097)} {
		if _, err := CompareDecimal([]byte(ref), nil, p); !errors.Is(err, ErrReference) {
			t.Errorf("bad answer did not fail preflight: length=%d, %v", len(ref), err)
		}
	}
	for _, config := range []DecimalPolicy{{}, {Absolute: "-0.01"}, {Relative: "Inf"}, {Absolute: "1e99999"}, {Absolute: "0", Text: []string{"1e513"}}, {Absolute: "0", Text: []string{"HERE", "HERE"}}} {
		if config.Valid() {
			t.Errorf("invalid policy accepted: %+v", config)
		}
	}
	if _, err := CompareDecimal([]byte("1"), make([]byte, decimalBytes+1), p); !errors.Is(err, ErrOutput) {
		t.Fatalf("byte limit: %v", err)
	}
}
