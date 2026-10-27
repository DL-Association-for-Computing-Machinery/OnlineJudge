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
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// ErrReportUnknown means acceptance is unconfirmed. Retain the original token
// and frozen result for an explicit retry; never rerun or change the result.
var ErrReportUnknown = errors.New("report acceptance unknown")

// Reporter sends frozen results to a single authenticated controller.
type Reporter struct{ transport *Renewer }

// NewReporter uses the same bounded TLS policy as lease renewal, with a separate
// connection budget so a slow report cannot block an active attempt's renewal.
func NewReporter(endpoint string, roots *x509.CertPool, cert tls.Certificate) (*Reporter, error) {
	r, err := NewRenewer(endpoint, roots, cert)
	if err != nil {
		return nil, err
	}
	r.endpoint = strings.TrimSuffix(r.endpoint, "/renew") + "/report"
	return &Reporter{transport: r}, nil
}

// Report performs one request, acknowledges only HTTP 204 and never redirects or
// automatically retries. Lease/node/conflict rejections are terminal; all other
// responses are conservatively unknown. Errors never contain response data.
func (r *Reporter) Report(ctx context.Context, token string, result submission.Result) error {
	if !tokenPattern.MatchString(token) || !tokenPattern.MatchString(result.EvidenceSHA256) {
		return submission.ErrInvalid
	}
	switch result.Verdict {
	case "AC", "WA", "CE", "RE", "TLE", "MLE", "OLE", "SE":
	default:
		return submission.ErrInvalid
	}
	body, _ := json.Marshal(struct {
		Token    string `json:"lease_token"`
		Verdict  string `json:"verdict"`
		Evidence string `json:"evidence_sha256"`
	}{token, result.Verdict, result.EvidenceSHA256})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.transport.endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrReportUnknown
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.transport.client.Do(req)
	if err != nil {
		return ErrReportUnknown
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return submission.ErrNode
	}
	if resp.StatusCode != http.StatusConflict {
		return ErrReportUnknown
	}
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/problem+json" {
		return ErrReportUnknown
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1025))
	if err != nil || len(data) > 1024 {
		return ErrReportUnknown
	}
	var problem struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(data, &problem) != nil {
		return ErrReportUnknown
	}
	switch problem.Code {
	case "LEASE_INVALID":
		return submission.ErrLease
	case "RESULT_CONFLICT":
		return submission.ErrResultConflict
	default:
		return ErrReportUnknown
	}
}

// Close releases idle connections when the owning worker shuts down.
func (r *Reporter) Close() { r.transport.Close() }
