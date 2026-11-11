package problemvalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
)

// ConvolutionSourceID is the only source admitted by the small differential trial.
const ConvolutionSourceID = "library-checker:convolution_mod"

const (
	convolutionOraclePath    = "convolution/convolution_mod/sol/naive.cpp"
	convolutionCandidatePath = "convolution/convolution_mod/sol/correct.cpp"
)

var errConvolutionSource = errors.New("fixed convolution sources unavailable or invalid")

// ConvolutionProgram identifies an exact compilation input without exposing it.
type ConvolutionProgram struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type convolutionProgram struct {
	identity ConvolutionProgram
	source   string
}

// ConvolutionSources retains only the two verified private compilation inputs.
// Its zero value is invalid.
type ConvolutionSources struct {
	revision  string
	oracle    convolutionProgram
	candidate convolutionProgram
}

// LoadConvolutionSources admits exactly two single-file programs from the
// reviewed source lock. It freezes the checked bytes before any slot starts.
func LoadConvolutionSources(root string, sourceLock []byte) (ConvolutionSources, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || len(sourceLock) > 1<<20 ||
		convolutionSHA256(sourceLock) != SourceLockSHA256 {
		return ConvolutionSources{}, errConvolutionSource
	}
	var lock struct {
		Commit string                        `json:"commit"`
		Files  map[string]ConvolutionProgram `json:"files"`
	}
	if json.Unmarshal(sourceLock, &lock) != nil || len(lock.Commit) != 40 {
		return ConvolutionSources{}, errConvolutionSource
	}
	load := func(path string) (convolutionProgram, error) {
		identity, ok := lock.Files[path]
		if !ok || identity.Size < 1 || identity.Size > 64<<10 {
			return convolutionProgram{}, errConvolutionSource
		}
		data, err := checkedfile.ReadPublic(filepath.Join(root, filepath.FromSlash(path)), identity.Size)
		if err != nil || int64(len(data)) != identity.Size || convolutionSHA256(data) != identity.SHA256 {
			return convolutionProgram{}, errConvolutionSource
		}
		identity.Path = path
		return convolutionProgram{identity: identity, source: string(data)}, nil
	}
	oracle, err := load(convolutionOraclePath)
	if err != nil {
		return ConvolutionSources{}, err
	}
	candidate, err := load(convolutionCandidatePath)
	if err != nil {
		return ConvolutionSources{}, err
	}
	return ConvolutionSources{revision: lock.Commit, oracle: oracle, candidate: candidate}, nil
}

func convolutionSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
