//go:build darwin || linux

package main

import (
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestMediumAndBulkContendForSameLock(t *testing.T) {
	medium, _ := workerhost.ReferenceSlotForTier(experiment.ReferenceMedium)
	bulk, _ := workerhost.ReferenceSlotForTier(experiment.ReferenceMediumBulk)
	dir := t.TempDir()
	first, err := acquireLock(filepath.Join(dir, medium.LockName()))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := acquireLock(filepath.Join(dir, bulk.LockName())); err == nil {
		second.Close()
		t.Fatal("same sandbox acquired through another tier")
	}
}
