package submission

import "context"

// TaskMaterials is an authorized immutable submission snapshot, never a public receipt.
type TaskMaterials struct {
	SubmissionID   string `json:"submission_id"`
	Version        string `json:"material_version"`
	ManifestSHA256 string `json:"manifest_sha256"`
	// Bytes preserve the exact canonical representation across JSON transport (base64).
	Manifest []byte `json:"manifest"`
	Language string `json:"language"`
	Source   string `json:"source"`
}

// MaterialStore authorizes an active node, current attempt, generations and lease
// before returning only that attempt's pinned source and material manifest.
type MaterialStore interface {
	Materials(context.Context, NodeIdentity, string) (TaskMaterials, error)
}
