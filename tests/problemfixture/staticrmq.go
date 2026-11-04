package problemfixture

import "testing"

// NewStaticRMQ supplies synthetic material under the fixed source identity.
func NewStaticRMQ(t *testing.T) *Bundle {
	t.Helper()
	return newReferenceSource(t, "library-checker:staticrmq", "data_structure/staticrmq")
}
