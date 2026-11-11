package problemvalidation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestSmallConvolutionCorpusHasStableBoundedInputs(t *testing.T) {
	cases := smallConvolutionCorpus()
	if len(cases) != 24 || string(cases[9].input) != "2 2\n1 2\n3 4\n" {
		t.Fatal("missing fixed hand-checked case or changed case count")
	}
	sum := sha256.New()
	seen := make(map[string]bool)
	for i, c := range cases {
		if len(c.input) > 512 || seen[string(c.input)] {
			t.Fatalf("case %d exceeded byte budget or was duplicated", i+1)
		}
		seen[string(c.input)] = true
		_, _ = sum.Write(c.input)
		in := bytes.NewReader(c.input)
		var a, b, coefficient int
		if _, err := fmt.Fscan(in, &a, &b); err != nil || a < 1 || a > 16 || b < 1 || b > 16 || a+b-1 != c.coefficients {
			t.Fatalf("case %d has invalid dimensions or expected output count", i+1)
		}
		for range a + b {
			if _, err := fmt.Fscan(in, &coefficient); err != nil || coefficient < 0 || coefficient >= 998244353 {
				t.Fatalf("case %d has an invalid input coefficient", i+1)
			}
		}
		if _, err := fmt.Fscan(in, &coefficient); err != io.EOF {
			t.Fatalf("case %d has extra input", i+1)
		}
	}
	const digest = "b4d0acee90e20829f5f79c65ee7eb4e5fc0d99308fe2b00387f930a0d2112576"
	if got := hex.EncodeToString(sum.Sum(nil)); got != digest {
		t.Fatalf("v1 input identity changed to %s", got)
	}
}

func TestConvolutionCoefficientsRequireCompleteFiniteFieldOutput(t *testing.T) {
	actual, err := convolutionCoefficients([]byte("\t0003\r\n10 8\n"), 3)
	if err != nil || !slices.Equal(actual, []uint32{3, 10, 8}) {
		t.Fatalf("valid integer output was not parsed: %v, %v", actual, err)
	}
	for name, output := range map[string]string{
		"empty":       "",
		"missing":     "3 10",
		"extra":       "3 10 8 0",
		"signed":      "-3 10 8",
		"plus":        "+3 10 8",
		"non-decimal": "3 1e1 8",
		"invalid-utf": "3\xff 10 8",
		"modulus":     "998244353 10 8",
		"overflow":    "9999999999999999999999 10 8",
		"over-budget": "3 10 8" + strings.Repeat(" ", 507),
	} {
		t.Run(name, func(t *testing.T) {
			if parsed, err := convolutionCoefficients([]byte(output), 3); err == nil || parsed != nil {
				t.Fatal("malformed evidence was admitted")
			}
		})
	}
	if got, err := convolutionCoefficients([]byte("0 998244352"), 2); err != nil || !slices.Equal(got, []uint32{0, 998244352}) {
		t.Fatalf("field boundaries were not accepted: %v, %v", got, err)
	}
}

func TestConvolutionEvidenceRequiresTheFixedFirstMismatch(t *testing.T) {
	p := ConvolutionPlan{Cases: 24}
	const input = "2 2\n1 2\n3 4\n"
	fresh := func() *ConvolutionCounterexample {
		return &ConvolutionCounterexample{Case: 10, Input: input, InputSHA256: convolutionSHA256([]byte(input)),
			Expected: []uint32{3, 10, 8}, Actual: []uint32{4, 10, 8}}
	}
	if !p.ValidOutcome("passed", 24, nil) || p.ValidOutcome("passed", 23, nil) ||
		p.ValidOutcome("passed", 24, fresh()) || !p.ValidOutcome("mismatch", 9, fresh()) {
		t.Fatal("determinate success/mismatch contract changed")
	}
	for name, edit := range map[string]func(*ConvolutionCounterexample){
		"wrong point":  func(c *ConvolutionCounterexample) { c.Case++ },
		"wrong digest": func(c *ConvolutionCounterexample) { c.InputSHA256 = strings.Repeat("a", 64) },
		"wrong input with valid digest": func(c *ConvolutionCounterexample) {
			c.Input = "1 1\n0\n0\n"
			c.InputSHA256 = convolutionSHA256([]byte(c.Input))
		},
		"wrong anchor": func(c *ConvolutionCounterexample) { c.Expected[0] = 5 },
		"out of field": func(c *ConvolutionCounterexample) { c.Actual[0] = 998244353 },
		"identical":    func(c *ConvolutionCounterexample) { c.Actual = c.Expected },
		"short output": func(c *ConvolutionCounterexample) { c.Actual = c.Actual[:2] },
	} {
		t.Run(name, func(t *testing.T) {
			c := fresh()
			edit(c)
			if p.ValidOutcome("mismatch", 9, c) {
				t.Fatal("invalid counterexample accepted")
			}
		})
	}
}
