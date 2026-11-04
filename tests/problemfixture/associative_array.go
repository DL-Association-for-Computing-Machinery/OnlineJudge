package problemfixture

import "testing"

// NewAssociativeArray binds synthetic materials to the fixed source identity.
func NewAssociativeArray(t *testing.T) *Bundle {
	t.Helper()
	return newReferenceSource(t, "library-checker:associative_array", "data_structure/associative_array")
}
