package workerclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// ErrClaimUnknown means a claim may have committed without a usable reply.
// Do not immediately claim again; recovery of the unobserved lease is a separate
// controller operation. This client neither polls nor retries.
var ErrClaimUnknown = errors.New("claim acceptance unknown")

// Acquirer claims one task and fetches its materials from one controller.
// Its connection budget is separate from renewal and reporting.
type Acquirer struct{ transport *Renewer }

// NewAcquirer requires explicit HTTPS trust and node credentials, with the same
// bounded transport and redirect refusal as NewRenewer.
func NewAcquirer(endpoint string, roots *x509.CertPool, cert tls.Certificate) (*Acquirer, error) {
	r, err := NewRenewer(endpoint, roots, cert)
	if err != nil {
		return nil, err
	}
	r.endpoint = strings.TrimSuffix(r.endpoint, "/renew")
	return &Acquirer{transport: r}, nil
}

var submissionIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func validLease(l submission.Lease) bool {
	return tokenPattern.MatchString(l.Token) && submissionIDPattern.MatchString(l.SubmissionID) &&
		l.JudgeGeneration > 0 && l.RecoveryGeneration > 0 && l.AttemptNumber > 0 && !l.Until.IsZero()
}

// Claim accepts only a complete lease or an explicit empty queue. Until is
// informational: execution must obtain a fresh renewal using LeasedRunner's
// monotonic budget, rather than trusting clocks shared across hosts.
func (a *Acquirer) Claim(ctx context.Context) (submission.Lease, error) {
	resp, err := a.post(ctx, "claim", struct{}{})
	if err != nil {
		return submission.Lease{}, ErrClaimUnknown
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return submission.Lease{}, submission.ErrNoTask
	case http.StatusUnauthorized, http.StatusForbidden:
		return submission.Lease{}, submission.ErrNode
	case http.StatusOK:
	default:
		return submission.Lease{}, ErrClaimUnknown
	}
	var value struct {
		Token    string    `json:"lease_token"`
		ID       string    `json:"submission_id"`
		Judge    int64     `json:"judge_generation"`
		Recovery int64     `json:"recovery_generation"`
		Attempt  int32     `json:"attempt_number"`
		Until    time.Time `json:"lease_until"`
	}
	if readJSON(resp, 1024, &value) != nil {
		return submission.Lease{}, ErrClaimUnknown
	}
	l := submission.Lease{Token: value.Token, SubmissionID: value.ID, JudgeGeneration: value.Judge,
		RecoveryGeneration: value.Recovery, AttemptNumber: value.Attempt, Until: value.Until}
	if !validLease(l) {
		return submission.Lease{}, ErrClaimUnknown
	}
	return l, nil
}

func (a *Acquirer) post(ctx context.Context, action string, value any) (*http.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("cannot construct acquisition request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.transport.endpoint+"/"+action, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("cannot construct acquisition request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.transport.client.Do(req)
	if err != nil {
		return nil, errors.New("acquisition transport unavailable")
	}
	return resp, nil
}

func readJSON(resp *http.Response, limit int64, value any) error {
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return errors.New("invalid acquisition media type")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(data)) > limit || json.Unmarshal(data, value) != nil {
		return errors.New("invalid or oversized acquisition response")
	}
	return nil
}

// Close releases idle connections when the owning worker shuts down.
func (a *Acquirer) Close() { a.transport.Close() }
