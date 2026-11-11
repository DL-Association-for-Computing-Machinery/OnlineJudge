package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

func TestReferenceClaimControllerOptIn(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	h, err := configureReferenceClaims(next, nil, nil, "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/worker/reference/claim", nil))
	if w.Code != http.StatusNotFound {
		t.Fatal("reference endpoint enabled without configuration")
	}
	if h, err := configureReferenceClaims(next, nil, nil, "invalid node", nil, false); h != nil || !errors.Is(err, problemreference.ErrInvalid) {
		t.Fatal("invalid reference node did not fail startup", err)
	}
}

func TestReferenceExecutionControllerOptIn(t *testing.T) {
	for _, tc := range []struct {
		value, node, root string
		enabled, invalid  bool
	}{
		{"", "worker", "/private", false, false},
		{"1", "worker", "/private", true, false},
		{"1", "", "/private", false, true},
		{"1", "worker", "", false, true},
		{"true", "worker", "/private", false, true},
	} {
		got, err := referenceExecutionEnabled(tc.value, tc.node, tc.root)
		if got != tc.enabled || (err != nil) != tc.invalid {
			t.Fatalf("invalid opt-in outcome: %+v (%v)", tc, err)
		}
	}
}
