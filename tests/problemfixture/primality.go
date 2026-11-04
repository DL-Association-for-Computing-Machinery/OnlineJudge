package problemfixture

import "testing"

// NewPrimality binds a small synthetic closure to the fixed primality layout.
// These bytes are test fixtures, not upstream reference-program evidence.
func NewPrimality(t *testing.T) *Bundle {
	t.Helper()
	b := newReferenceSource(t, "library-checker:primality_test", "number_theory/primality_test")
	for name, content := range map[string]string{
		"number_theory/primality_test/sol/correct.cpp":    "#include \"../lib/prime.hpp\"\nint main(){return value()!=3;}\n",
		"number_theory/primality_test/lib/prime.hpp":      "#include \"montgomery.hpp\"\n",
		"number_theory/primality_test/lib/montgomery.hpp": "#include \"util.hpp\"\n",
		"number_theory/primality_test/lib/util.hpp":       "#include \"random.h\"\n",
		"common/random.h": "// 原始 UTF-8 字节\ninline int value(){return 3;}\n",
	} {
		b.AddOriginal(t, name, []byte(content))
	}
	return b
}
