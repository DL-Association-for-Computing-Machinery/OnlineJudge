package problemvalidation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
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
	draft     ConvolutionDraftBinding
}

type convolutionLock struct {
	Commit string                        `json:"commit"`
	Files  map[string]ConvolutionProgram `json:"files"`
}

func readConvolutionPins(sourceLock []byte) (string, ConvolutionProgram, ConvolutionProgram, error) {
	var lock convolutionLock
	if len(sourceLock) > 1<<20 || convolutionSHA256(sourceLock) != SourceLockSHA256 ||
		json.Unmarshal(sourceLock, &lock) != nil || len(lock.Commit) != 40 {
		return "", ConvolutionProgram{}, ConvolutionProgram{}, errConvolutionSource
	}
	oracle, candidate := lock.Files[convolutionOraclePath], lock.Files[convolutionCandidatePath]
	oracle.Path, candidate.Path = convolutionOraclePath, convolutionCandidatePath
	if oracle.Size < 1 || oracle.Size > 64<<10 || candidate.Size < 1 || candidate.Size > 64<<10 ||
		oracle.SHA256 == candidate.SHA256 {
		return "", ConvolutionProgram{}, ConvolutionProgram{}, errConvolutionSource
	}
	return lock.Commit, oracle, candidate, nil
}

// LoadConvolutionSources admits exactly two single-file programs from the
// reviewed source lock. It freezes the checked bytes before any slot starts.
func LoadConvolutionSources(root string, sourceLock []byte) (ConvolutionSources, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return ConvolutionSources{}, errConvolutionSource
	}
	revision, oracle, candidate, err := readConvolutionPins(sourceLock)
	if err != nil {
		return ConvolutionSources{}, err
	}
	load := func(identity ConvolutionProgram) (convolutionProgram, error) {
		data, err := checkedfile.ReadPublic(filepath.Join(root, filepath.FromSlash(identity.Path)), identity.Size)
		if err != nil || int64(len(data)) != identity.Size || convolutionSHA256(data) != identity.SHA256 {
			return convolutionProgram{}, errConvolutionSource
		}
		return convolutionProgram{identity: identity, source: string(data)}, nil
	}
	oracleProgram, err := load(oracle)
	if err != nil {
		return ConvolutionSources{}, err
	}
	candidateProgram, err := load(candidate)
	if err != nil {
		return ConvolutionSources{}, err
	}
	return ConvolutionSources{revision: revision, oracle: oracleProgram, candidate: candidateProgram}, nil
}

// FreezeDraftConvolution binds the reviewed solver bytes to one exact persisted
// draft. The caller supplies a trusted import identity and remains responsible
// for live authorization; freezing does not grant execution or admit a report.
func FreezeDraftConvolution(ctx context.Context, draft problem.Draft, id, owner string, expected problem.ImportIdentity, store problem.ReferenceObjectReader, sourceLock []byte) (ConvolutionSources, error) {
	revision, oracle, candidate, err := readConvolutionPins(sourceLock)
	if err != nil || expected.SourceID != ConvolutionSourceID || expected.SourceLockSHA256 != SourceLockSHA256 {
		return ConvolutionSources{}, errConvolutionSource
	}
	pins := problem.ConvolutionSolverObjects{
		Oracle:    materialstore.Object{SHA256: oracle.SHA256, Size: oracle.Size},
		Candidate: materialstore.Object{SHA256: candidate.SHA256, Size: candidate.Size},
	}
	programs, err := draft.FreezeConvolutionSources(ctx, id, owner, expected, pins, store)
	if err != nil {
		return ConvolutionSources{}, err
	}
	return ConvolutionSources{
		revision:  revision,
		oracle:    convolutionProgram{identity: oracle, source: programs.Oracle},
		candidate: convolutionProgram{identity: candidate, source: programs.Candidate},
		draft:     ConvolutionDraftBinding{ID: id, Version: expected.Version, ImportRevision: expected.ImportRevision},
	}, nil
}

func convolutionSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
