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
	h, err := configureReferenceClaims(next, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/worker/reference/claim", nil))
	if w.Code != http.StatusNotFound {
		t.Fatal("reference endpoint enabled without configuration")
	}
	if h, err := configureReferenceClaims(next, nil, nil, "invalid node"); h != nil || !errors.Is(err, problemreference.ErrInvalid) {
		t.Fatal("invalid reference node did not fail startup", err)
	}
}
