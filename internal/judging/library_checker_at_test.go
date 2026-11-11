package judging_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
)

func frozen(raw []byte) judging.FrozenText {
	return judging.FrozenText{Reader: bytes.NewReader(raw), Size: int64(len(raw))}
}

func requireSliceParity(t *testing.T, ref, out []byte, limits judging.TextLimits) {
	t.Helper()
	want, wantErr := judging.CompareLibraryCheckerText(ref, out, limits)
	got, gotErr := judging.CompareLibraryCheckerTextAt(t.Context(), frozen(ref), frozen(out), limits)
	if got != want || !errors.Is(gotErr, wantErr) {
		t.Fatalf("stream (%v, %v) differs from slice (%v, %v), byte lengths %d/%d", got, gotErr, want, wantErr, len(ref), len(out))
	}
}

func TestFrozenCheckerMatchesSliceDomain(t *testing.T) {
	limits := judging.TextLimits{Bytes: 1024, TokenBytes: 12}
	for _, test := range [][2][]byte{
		{nil, []byte(" \t\n\r")}, {[]byte("�\x00a\vB\fc"), []byte("\r�\x00a\vB\fc\n")},
		{[]byte("YES 01"), []byte("yes 1")}, {[]byte("x"), []byte("y abcdefghijklm \xff")},
		{[]byte("x"), []byte("y abcdefghijklm")}, {[]byte("x \xff"), bytes.Repeat([]byte("1"), 1025)},
		{[]byte("x"), append(bytes.Repeat([]byte(" "), 1024), 255)},
	} {
		requireSliceParity(t, test[0], test[1], limits)
	}
	rng := rand.New(rand.NewPCG(37, 73))
	alphabet := []byte("abc 0\t\r\n\v\f\x00\xff\xc3\xa9")
	for i := 0; i < 500; i++ {
		raw := func() []byte {
			result := make([]byte, rng.IntN(96))
			for j := range result {
				result[j] = alphabet[rng.IntN(len(alphabet))]
			}
			return result
		}
		ref, out := raw(), raw()
		if i%3 == 0 {
			out = append(append([]byte(" \t"), ref...), '\n')
		}
		requireSliceParity(t, ref, out, judging.TextLimits{Bytes: 1 + rng.IntN(128), TokenBytes: 1 + rng.IntN(12)})
	}
}

func TestFrozenCheckerCrossesReadAndTokenBoundaries(t *testing.T) {
	limit := 64 << 10
	for _, split := range []int{0, 1, 2, 3, 4} {
		// Both valid four-byte UTF-8 and invalid tails cross the 16 KiB read.
		ref := append(bytes.Repeat([]byte(" "), (16<<10)-split), []byte("𐍈\t�")...)
		out := []byte("𐍈 �")
		requireSliceParity(t, ref, out, judging.TextLimits{Bytes: limit, TokenBytes: 4})
		ref[len(ref)-1] = 0xff
		requireSliceParity(t, ref, out, judging.TextLimits{Bytes: limit, TokenBytes: 4})
	}
	ref := bytes.Repeat([]byte("a"), limit)
	for _, cap := range []int{limit, limit - 1} {
		requireSliceParity(t, ref, ref, judging.TextLimits{Bytes: limit, TokenBytes: cap})
	}
}

type readerAtFunc func([]byte, int64) (int, error)

func (f readerAtFunc) ReadAt(p []byte, off int64) (int, error) { return f(p, off) }

func TestFrozenCheckerRejectsShortReadAndDoesNotTouchAdjacentBytes(t *testing.T) {
	limits := judging.TextLimits{Bytes: 32, TokenBytes: 4}
	ref := frozen([]byte("1"))
	for _, e := range []error{nil, io.EOF, errors.New("private disk detail")} {
		short := judging.FrozenText{Size: 2, Reader: readerAtFunc(func(p []byte, _ int64) (int, error) {
			p[0] = '1'
			return 1, e
		})}
		ok, err := judging.CompareLibraryCheckerTextAt(t.Context(), ref, short, limits)
		if ok || !errors.Is(err, judging.ErrTextRead) || strings.Contains(err.Error(), "private") {
			t.Fatalf("short read produced %v, %v", ok, err)
		}
	}
	reads := 0
	exact := judging.FrozenText{Size: 1, Reader: readerAtFunc(func(p []byte, off int64) (int, error) {
		reads++
		if off != 0 || len(p) != 1 {
			t.Fatalf("out-of-span read at %d for %d bytes", off, len(p))
		}
		p[0] = '1'
		return 1, io.EOF // permitted when the full requested span was read
	})}
	if ok, err := judging.CompareLibraryCheckerTextAt(t.Context(), exact, exact, limits); !ok || err != nil || reads != 4 {
		t.Fatalf("exact final EOF was not accepted: %v, %v, reads=%d", ok, err, reads)
	}
	unreadable := judging.FrozenText{Size: 1, Reader: readerAtFunc(func([]byte, int64) (int, error) {
		t.Fatal("invalid reference must not touch contestant output")
		return 0, nil
	})}
	if _, err := judging.CompareLibraryCheckerTextAt(t.Context(), frozen([]byte{0xff}), unreadable, limits); !errors.Is(err, judging.ErrReference) {
		t.Fatal(err)
	}
}

func TestFrozenCheckerCancellationDuringFinalReadOrRewind(t *testing.T) {
	for _, stopAt := range []int{1, 2} {
		t.Run(map[int]string{1: "domain scan", 2: "token pass"}[stopAt], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reads := 0
			ref := judging.FrozenText{Size: 1, Reader: readerAtFunc(func(p []byte, _ int64) (int, error) {
				reads++
				p[0] = '1'
				if reads == stopAt {
					cancel()
				}
				return 1, nil
			})}
			ok, err := judging.CompareLibraryCheckerTextAt(ctx, ref, frozen([]byte("1")), judging.TextLimits{Bytes: 4, TokenBytes: 2})
			if ok || !errors.Is(err, context.Canceled) {
				t.Fatalf("read %d produced %v, %v", stopAt, ok, err)
			}
		})
	}
}

type patternedText struct{}

func (patternedText) ReadAt(p []byte, off int64) (int, error) {
	for i := range p {
		p[i] = "1 "[(off+int64(i))%2]
	}
	return len(p), nil
}

func BenchmarkFrozenCheckerScratch(b *testing.B) {
	for _, pair := range []struct {
		name string
		size int64
	}{{"64KiB", 64 << 10}, {"16MiB", 16 << 20}} {
		b.Run(pair.name, func(b *testing.B) {
			text := judging.FrozenText{Reader: patternedText{}, Size: pair.size}
			limits := judging.TextLimits{Bytes: int(pair.size), TokenBytes: 64 << 10}
			b.ReportAllocs()
			b.SetBytes(pair.size * 2)
			for b.Loop() {
				if ok, err := judging.CompareLibraryCheckerTextAt(b.Context(), text, text, limits); !ok || err != nil {
					b.Fatal(ok, err)
				}
			}
		})
	}
}
