package problemvalidation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

const (
	convolutionModulus    = 998244353
	convolutionDimensions = 16
	convolutionCases      = 24
	convolutionCaseBytes  = 512
	convolutionCorpusID   = "convolution-mod-small-v1"
	convolutionCorpusSeed = uint64(0x4f4a2d50342d3031)
)

type convolutionCase struct {
	input        []byte
	coefficients int
}

func convolutionCorpusDigest(cases []convolutionCase) string {
	sum := sha256.New()
	for _, c := range cases {
		_, _ = sum.Write(c.input)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// The corpus is independent of the imported official tests and their answers.
// Freeze the generator and seed together; a changed input sequence needs a new ID.
func smallConvolutionCorpus() []convolutionCase {
	cases := make([]convolutionCase, 0, convolutionCases)
	add := func(a, b []uint32) {
		raw := strconv.AppendInt(nil, int64(len(a)), 10)
		raw = strconv.AppendInt(append(raw, ' '), int64(len(b)), 10)
		for _, row := range [][]uint32{a, b} {
			raw = append(raw, '\n')
			for i, v := range row {
				if i > 0 {
					raw = append(raw, ' ')
				}
				raw = strconv.AppendUint(raw, uint64(v), 10)
			}
		}
		cases = append(cases, convolutionCase{append(raw, '\n'), len(a) + len(b) - 1})
	}
	boundary := []uint32{0, 1, convolutionModulus - 1}
	for _, a := range boundary {
		for _, b := range boundary {
			add([]uint32{a}, []uint32{b})
		}
	}
	add([]uint32{1, 2}, []uint32{3, 4})
	add([]uint32{convolutionModulus - 1, convolutionModulus - 1}, []uint32{convolutionModulus - 1, convolutionModulus - 1})
	add([]uint32{convolutionModulus - 1, 1}, []uint32{1, 1})
	add([]uint32{0, 0, 1}, []uint32{2, 0, 3})
	add(make([]uint32, convolutionDimensions), []uint32{convolutionModulus - 1, 7})
	a, b := make([]uint32, convolutionDimensions), make([]uint32, convolutionDimensions)
	a[len(a)-1], b[0], b[len(b)-1] = 1, 1, convolutionModulus-1
	add(a, b)

	// Hashing fixed-width counters makes the sequence independent of Go's
	// random generator implementation and of the order in which cases run.
	var counter [16]byte
	binary.BigEndian.PutUint64(counter[:8], convolutionCorpusSeed)
	var index uint64
	next := func(mod uint32) uint32 {
		binary.BigEndian.PutUint64(counter[8:], index)
		index++
		sum := sha256.Sum256(counter[:])
		return binary.BigEndian.Uint32(sum[:4]) % mod
	}
	for len(cases) < convolutionCases {
		a, b := make([]uint32, next(convolutionDimensions)+1), make([]uint32, next(convolutionDimensions)+1)
		for _, row := range [][]uint32{a, b} {
			for i := range row {
				row[i] = next(convolutionModulus)
			}
		}
		add(a, b)
	}
	return cases
}

var errConvolutionOutput = errors.New("invalid small convolution output")

// Parse both solvers' evidence identically. Extra coefficients, non-decimal
// tokens, and invalid field elements must fail before comparing the values.
func convolutionCoefficients(raw []byte, want int) ([]uint32, error) {
	if len(raw) == 0 || len(raw) > experiment.ConvolutionComparisonOutputBytes || want < 1 || want > 2*convolutionDimensions-1 {
		return nil, errConvolutionOutput
	}
	values := make([]uint32, 0, want)
	for i := 0; i < len(raw); {
		if convolutionSpace(raw[i]) {
			i++
			continue
		}
		if len(values) == want {
			return nil, errConvolutionOutput
		}
		var v uint64
		for i < len(raw) && !convolutionSpace(raw[i]) {
			c := raw[i]
			if c < '0' || c > '9' {
				return nil, errConvolutionOutput
			}
			v = v*10 + uint64(c-'0')
			if v >= convolutionModulus {
				return nil, errConvolutionOutput
			}
			i++
		}
		values = append(values, uint32(v))
	}
	if len(values) != want {
		return nil, errConvolutionOutput
	}
	return values, nil
}

func convolutionSpace(c byte) bool {
	return c == ' ' || c >= '\t' && c <= '\r'
}
