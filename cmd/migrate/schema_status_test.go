package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestSchemaStatusRefusesInvalidConfirmation(t *testing.T) {
	for _, args := range [][]string{nil, {"db"}, {"", "public"}, {"db", ""}, {"db", strings.Repeat("a", 64)}, {"db", "public\x00other"}} {
		var output bytes.Buffer
		err := runSchemaStatus(args, func(string) string {
			t.Fatal("invalid confirmation read database configuration")
			return ""
		}, &output)
		if !errors.Is(err, errSchemaStatusUsage) || output.Len() != 0 {
			t.Fatalf("unexpected result for invalid confirmation: %v", err)
		}
	}
}
