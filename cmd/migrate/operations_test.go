package main

import (
	"bytes"
	"errors"
	"testing"
)

func TestJudgeSnapshotRefusesInvalidWindowBeforeDatabaseAccess(t *testing.T) {
	for _, args := range [][]string{nil, {"db"}, {"", "10"}, {"db", "0"}, {"db", "61"}, {"db", "+1"}, {"db", "01"}} {
		var out bytes.Buffer
		err := runJudgeSnapshot(args, func(string) string {
			t.Fatal("bad arguments reached database configuration")
			return ""
		}, &out)
		if !errors.Is(err, errJudgeSnapshotUsage) || out.Len() != 0 {
			t.Fatalf("invalid arguments returned unexpected result: %v", err)
		}
	}
}
