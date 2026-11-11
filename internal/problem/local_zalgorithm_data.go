package problem

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"

const (
	zAlgorithmLocalData    = "oj-local:zalgorithm-scale-v1"
	zAlgorithmLocalVersion = "965239246cbdbd63f1641cfcf551d9c8f3a65cd558e194668ad845cac1e39711"
	zAlgorithmSource       = "library-checker:zalgorithm"
	zAlgorithmSourceLock   = "fe2722edade748228c9add9c54704c09861fa6be649b5cdf59e1f214b5ee9617"
	zAlgorithmUpstream     = 29
)

// Only this reviewed revision has local cases. The ordered upstream catalogue
// is still checked against the upstream definition and hashes independently.
var zAlgorithmLocalCases = [...]ReferenceCase{
	{
		ID:     "case-0030",
		Input:  materialstore.Object{SHA256: "87428fc522803d31065e7bce3cf03fe475096631e5e07bbd7a0fde60c4cf25c7", Size: 2},
		Answer: materialstore.Object{SHA256: "4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865", Size: 2},
	},
	{
		ID:     "case-0031",
		Input:  materialstore.Object{SHA256: "069af87ff42316e1bbe6cc65d3ed71a71cdcf9cc972257c93a7d934b435c8ad2", Size: 500001},
		Answer: materialstore.Object{SHA256: "9a3768bb91a7c39e5777767f091605ebb81cac9a047140ba7832bc5f067ab434", Size: 3388895},
	},
}

// These objects are also pinned by the source lock on the API side.
var (
	zAlgorithmLocalSource = materialstore.Object{SHA256: "9f7168d28138fb61d7706b5b899bb0ae21503a848428fc7d7551c5a8507ff8d7", Size: 969}
	zAlgorithmDefinition  = materialstore.Object{SHA256: "75d80d297a4f12c13c828b289a4e5795d8573d1a668ad07e4f19c81dcc54fc4c", Size: 499}
	zAlgorithmHashes      = materialstore.Object{SHA256: "45ec72478cb3d175a65be662df6dc2e2ada46f6fd146f796dd07ea3174a8cf84", Size: 5099}
)

func validDocumentCaseMapping(d document) bool {
	if d.LocalData == "" {
		return len(d.Provenance.Mapping) == len(d.TestCases)
	}
	if d.LocalData != zAlgorithmLocalData || d.Provenance.SourceID != zAlgorithmSource ||
		d.Provenance.SourceLockSHA256 != zAlgorithmSourceLock || d.Version != zAlgorithmLocalVersion ||
		len(d.Provenance.Mapping) != zAlgorithmUpstream || len(d.TestCases) != zAlgorithmUpstream+len(zAlgorithmLocalCases) {
		return false
	}
	for i, pinned := range zAlgorithmLocalCases {
		c := d.TestCases[zAlgorithmUpstream+i]
		if c.Sample || c.ID != pinned.ID || c.Input.object() != pinned.Input || c.Answer.object() != pinned.Answer {
			return false
		}
	}
	return true
}

// upstreamPlanSize separates the exact local suffix without changing the plan
// encoding or accepting an unrecognized extension in the worker.
func upstreamPlanSize(p ReferencePlan) (int, bool) {
	if p.Version != zAlgorithmLocalVersion {
		return len(p.Cases), true
	}
	if p.SourceID != zAlgorithmSource || p.SourceLockSHA256 != zAlgorithmSourceLock ||
		p.Source != zAlgorithmLocalSource || p.Definition != zAlgorithmDefinition || p.Hashes != zAlgorithmHashes ||
		len(p.Cases) != zAlgorithmUpstream+len(zAlgorithmLocalCases) {
		return 0, false
	}
	for i, pinned := range zAlgorithmLocalCases {
		c := p.Cases[zAlgorithmUpstream+i]
		if c.ID != pinned.ID || c.Input != pinned.Input || c.Answer != pinned.Answer {
			return 0, false
		}
	}
	return zAlgorithmUpstream, true
}
