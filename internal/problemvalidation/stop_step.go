package problemvalidation

// StopStep is the owner-copied host projection; a CLI start is not a kill receipt.
type StopStep struct {
	Stage             string `json:"stage"`
	Code              string `json:"code"`
	ElapsedMS         int64  `json:"elapsed_ms"`
	ElapsedCapped     bool   `json:"elapsed_capped,omitempty"`
	Count             int    `json:"count,omitempty"`
	ContextBefore     string `json:"context_before,omitempty"`
	CallAttempted     bool   `json:"call_attempted,omitempty"`
	LocalProcessStart string `json:"local_process_start,omitempty"`
}
