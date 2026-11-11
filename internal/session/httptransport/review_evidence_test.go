package httptransport_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type unavailableEvidence struct{ err error }

func (e unavailableEvidence) EvidenceForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewEvidence, error) {
	return problemreference.ReviewEvidence{}, e.err
}

func TestReviewEvidenceSessionFailurePriority(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{
		{identity.ErrConflict, 401},
		{errors.Join(identity.ErrConflict, identity.ErrStore), 503},
		{errors.Join(identity.ErrConflict, context.Canceled), 503},
	} {
		h, err := transport.WithReviewEvidence(draftHandler(t, &draftUseCase{}), unavailableEvidence{tc.err}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		r := request(http.MethodGet, "/api/v1/problem-review/"+draftID+"/evidence")
		r.Header.Set("X-OJ-Expected-Subject", strings.Repeat("e", 32))
		response(t, h, r, tc.code, false)
	}
}
