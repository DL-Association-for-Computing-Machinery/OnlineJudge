package judging

import (
	"errors"
	"testing"
)

func TestCheckerResultProtocolIsExplicit(t *testing.T) {
	for _, p := range []CheckerProtocol{0, CheckerICPC, CheckerTestlib0945} {
		for code := 0; code < 256; code++ {
			want := CheckerSystemError
			switch {
			case p == CheckerICPC && code == 42, p == CheckerTestlib0945 && code == 50:
				want = CheckerAccept
			case p == CheckerICPC && code == 43, p == CheckerTestlib0945 && code >= 51 && code <= 54:
				want = CheckerWrongAnswer
			}
			got, err := DecodeCheckerExit(p, true, code)
			if got != want || (want == CheckerSystemError && !errors.Is(err, ErrCheckerResult)) ||
				(want != CheckerSystemError && err != nil) {
				t.Fatalf("protocol %d, exit %d: got %v, %v; want %v", p, code, got, err, want)
			}
			if got, err := DecodeCheckerExit(p, false, code); got != CheckerSystemError || !errors.Is(err, ErrCheckerResult) {
				t.Fatalf("fault with protocol %d and exit %d: got %v, %v", p, code, got, err)
			}
		}
	}
}
