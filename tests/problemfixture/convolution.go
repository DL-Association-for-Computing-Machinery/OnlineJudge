package problemfixture

import "testing"

// NewConvolution binds synthetic material to the fixed convolution source.
func NewConvolution(t *testing.T) *Bundle {
	t.Helper()
	return newReferenceSource(t, "library-checker:convolution_mod", "convolution/convolution_mod")
}
