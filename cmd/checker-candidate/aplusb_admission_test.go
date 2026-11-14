package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAPlusBAdmissionCommandFailuresEmitNoOutput(t *testing.T) {
	target := []string{"review-aplusb-admission", "--node", "review-node",
		"--revision", strings.Repeat("a", 40), "--runtime", strings.Repeat("b", 64),
		"--slot", "oj-reference-sandbox"}
	for _, extras := range [][]string{
		nil,
		{"--receipt-directory", "relative-receipt", "--target-observation", "/runtime", "--checker-observation", "/checker"},
		{"--receipt-directory", "/receipt", "--target-observation", "/runtime", "--checker-observation", "/checker"},
		{"--receipt-directory", "/receipt", "--target-observation", "/runtime", "--checker-observation", "/checker", "--remaining"},
	} {
		var out, diagnostics bytes.Buffer
		if err := run(t.Context(), append(target, extras...), &out, &diagnostics); err == nil || out.Len() != 0 {
			t.Fatalf("incomplete invocation produced output: %v", err)
		}
	}
}
