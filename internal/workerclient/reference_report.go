package workerclient

import (
	"context"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// ReportReference sends one frozen evidence envelope. Only 204 acknowledges
// durable acceptance; retrying an unknown outcome must reuse the exact input.
func (a *Acquirer) ReportReference(ctx context.Context, input problemreference.WorkerReportInput) error {
	if a == nil || ctx == nil {
		return problemreference.ErrInvalid
	}
	if _, err := input.DecodeEvidence(); err != nil {
		return err
	}
	resp, err := a.post(ctx, "reference/report", input)
	if err != nil {
		return ErrReportUnknown
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusNoContent:
		if ctx.Err() == nil {
			return nil
		}
	case http.StatusBadRequest:
		return problemreference.ErrInvalid
	case http.StatusUnauthorized, http.StatusForbidden:
		return submission.ErrNode
	case http.StatusConflict:
		return problemreference.ErrClaimUnavailable
	}
	return ErrReportUnknown
}
