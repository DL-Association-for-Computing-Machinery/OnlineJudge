package password

import (
	"crypto/rand"
	"crypto/subtle"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	formatVersion = 1
	algorithm     = "argon2id"
)

// Record is the complete verification material, never a log or HTTP response.
// It is a structured internal contract, not a PHC string or a storage encoding.
// Callers must not mutate password or record slices during an operation.
type Record struct {
	FormatVersion uint32
	Algorithm     string
	ArgonVersion  uint32
	Parameters    Parameters
	Salt          []byte
	Digest        []byte
}

// Verification reports a comparison and, only on success, a policy migration hint.
// It never authorizes an account, changes credentials, or grants a session.
type Verification struct {
	Match        bool
	NeedsUpgrade bool
}

// Hasher holds an immutable policy. Use New; its zero value fails closed.
// Calls are synchronous: a caller's timeout cannot stop an in-flight derivation.
// Before network use, the caller must bound admission across all API processes
// and retain the calculation slot until the actual call returns.
type Hasher struct {
	current Parameters
	legacy  map[Parameters]struct{}
	limits  Limits
	salt    func([]byte) error
	derive  func([]byte, []byte, Parameters) []byte
}

// New validates and copies the full policy before any password work. Limits
// must come from an approved execution budget; this API does not choose one.
func New(policy Policy, limits Limits) (*Hasher, error) {
	if !limits.valid() || !policy.Current.valid(limits) {
		return nil, ErrPolicy
	}
	legacy := make(map[Parameters]struct{}, len(policy.Legacy))
	for _, p := range policy.Legacy {
		if !p.valid(limits) || p == policy.Current {
			return nil, ErrPolicy
		}
		if _, duplicate := legacy[p]; duplicate {
			return nil, ErrPolicy
		}
		legacy[p] = struct{}{}
	}
	return &Hasher{
		current: policy.Current, legacy: legacy, limits: limits,
		salt: func(dst []byte) error {
			_, err := io.ReadFull(rand.Reader, dst)
			return err
		},
		derive: func(password, salt []byte, p Parameters) []byte {
			return argon2.IDKey(password, salt, p.Iterations, p.MemoryKiB, p.Parallelism, p.DigestBytes)
		},
	}, nil
}

func (h *Hasher) validatePassword(password []byte) error {
	if h == nil || h.derive == nil || h.salt == nil {
		return ErrPolicy
	}
	if len(password) == 0 || uint64(len(password)) > uint64(h.limits.MaxPasswordBytes) {
		return ErrPassword
	}
	return nil
}

// Create uses a fresh cryptographically random salt and exactly Current.
// Passwords are raw bytes: no trimming, normalization, truncation, or product
// strength policy is performed. No complete memory-erasure guarantee is made.
func (h *Hasher) Create(password []byte) (Record, error) {
	if err := h.validatePassword(password); err != nil {
		return Record{}, err
	}
	p := h.current
	salt := make([]byte, p.SaltBytes)
	if err := h.salt(salt); err != nil {
		return Record{}, ErrRandom
	}
	return Record{
		FormatVersion: formatVersion, Algorithm: algorithm, ArgonVersion: argon2.Version,
		Parameters: p, Salt: salt, Digest: h.derive(password, salt, p),
	}, nil
}

// Verify rejects malformed or unapproved records before deriving a key. A wrong
// password returns a zero Verification and no error. Only the fixed-size digest
// comparison is constant time; the whole operation is not an anti-enumeration
// HTTP workflow. NeedsUpgrade is a hint, with no rehash or database side effect.
func (h *Hasher) Verify(password []byte, record Record) (Verification, error) {
	if err := h.validatePassword(password); err != nil {
		return Verification{}, err
	}
	p := record.Parameters
	if record.FormatVersion != formatVersion || record.Algorithm != algorithm ||
		record.ArgonVersion != argon2.Version || !p.valid(h.limits) ||
		uint64(len(record.Salt)) != uint64(p.SaltBytes) || uint64(len(record.Digest)) != uint64(p.DigestBytes) {
		return Verification{}, ErrRecord
	}
	_, legacy := h.legacy[p]
	if p != h.current && !legacy {
		return Verification{}, ErrRecord
	}
	// Derive from a fully matched policy tuple, never unchecked record fields.
	derived := h.derive(password, record.Salt, p)
	match := subtle.ConstantTimeCompare(derived, record.Digest) == 1
	clear(derived)
	return Verification{Match: match, NeedsUpgrade: match && legacy}, nil
}
