package experiment

// ReferenceFilesProfile bounds the explicit P4 file-cache experiment. These
// values are independent of M0's JSON transport and contestant resource limits.
type ReferenceFilesProfile struct {
	ID           string `json:"id"`
	SourceBytes  int64  `json:"source_bytes"`
	FileBytes    int64  `json:"file_bytes"`
	LiveBytes    int64  `json:"live_bytes"`
	PayloadBytes int64  `json:"payload_bytes"`
	Operations   int    `json:"operations"`
	Objects      int    `json:"objects"`
}

// PinnedReferenceFiles returns the reviewed protocol caps, not a capacity proof.
// Multipart storage, cache, collectors and cgroup peaks require real validation.
func PinnedReferenceFiles() ReferenceFilesProfile {
	return ReferenceFilesProfile{ID: "p4-gojudge-cached-files-v1", SourceBytes: 64 << 10, FileBytes: 48 << 20, LiveBytes: 64 << 20, PayloadBytes: 128 << 20, Operations: 512, Objects: 4}
}
