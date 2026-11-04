package problemfixture

import "testing"

// NewStaticRangeSum binds synthetic materials to the fixed source identity.
func NewStaticRangeSum(t *testing.T) *Bundle {
	t.Helper()
	return newReferenceSource(t, "library-checker:static_range_sum", "data_structure/static_range_sum")
}
