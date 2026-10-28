package workerhost

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestFixedSlots(t *testing.T) {
	for _, raw := range []string{"", "primary", "secondary"} {
		slot, err := ParseM0Slot(raw)
		if err != nil {
			t.Fatal(err)
		}
		wantContainer, wantLock := "oj-m0-sandbox", "slot.lock"
		if raw == "secondary" {
			wantContainer, wantLock = "oj-m0-sandbox-secondary", "slot-secondary.lock"
		}
		if slot.ContainerName() != wantContainer || slot.LockName() != wantLock {
			t.Fatal("slot mapping changed")
		}
	}
	for _, raw := range []string{"../slot", "PRIMARY", "primary ", "oj-m0-sandbox", strings.Repeat("a", 64)} {
		if _, err := ParseM0Slot(raw); err == nil {
			t.Fatal("arbitrary alias accepted")
		}
	}
}

func TestSlotStopIsolation(t *testing.T) {
	primary, a := opened(t)
	slot, _ := ParseM0Slot("secondary")
	b := &fakeHost{s: goodState(), response: "{}\n200"}
	b.s.ID = strings.Repeat("b", 64)
	secondary, err := openSlot(context.Background(), b.run, slot)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.calls[0][len(b.calls[0])-1]; got != slot.ContainerName() {
		t.Fatal("wrong initial container")
	}
	if err := secondary.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !a.s.Running || b.s.Running {
		t.Fatal("stop crossed slot boundary")
	}
	r, _ := http.NewRequest("GET", "http://127.0.0.1:5050/version", nil)
	response, err := primary.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	for _, call := range b.calls[1:] {
		for _, arg := range call {
			if arg == a.s.ID || arg == container {
				t.Fatal("secondary operation addressed primary")
			}
		}
	}
}
