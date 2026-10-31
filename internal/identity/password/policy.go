// Package password creates and verifies bounded, versioned Argon2id records.
// It has no production cost defaults, persistence, HTTP, or session side effects.
package password

import "errors"

var (
	ErrPolicy   = errors.New("invalid password policy")
	ErrPassword = errors.New("invalid password length")
	ErrRecord   = errors.New("invalid or unsupported password record")
	ErrRandom   = errors.New("password salt generation failed")
)

// Parameters is one exact supported cost and output tuple. MemoryKiB is KiB,
// Iterations is the pass count, and Parallelism is the Argon2 lane count.
// The structural minima below do not constitute production security approval.
type Parameters struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltBytes   uint32
	DigestBytes uint32
}

// Policy separates the current write tuple from explicitly allowed old tuples.
// Legacy means migration to Current is desired, not that every cost increases.
type Policy struct {
	Current Parameters
	Legacy  []Parameters
}

// Limits is the caller's explicit per-operation ceiling, not a concurrency or
// process memory budget. All fields are required; zero never selects a default.
// Changing MaxPasswordBytes must account for passwords accepted by older policy.
type Limits struct {
	MaxMemoryKiB     uint32
	MaxIterations    uint32
	MaxParallelism   uint8
	MaxPasswordBytes uint32
	MaxSaltBytes     uint32
	MaxDigestBytes   uint32
}

func (l Limits) valid() bool {
	maxInt := uint64(^uint(0) >> 1)
	return l.MaxMemoryKiB > 0 && uint64(l.MaxMemoryKiB)*1024 <= maxInt &&
		l.MaxIterations > 0 && l.MaxParallelism > 0 &&
		l.MaxPasswordBytes > 0 && uint64(l.MaxPasswordBytes) <= maxInt &&
		l.MaxSaltBytes >= 8 && uint64(l.MaxSaltBytes) <= maxInt &&
		l.MaxDigestBytes >= 16 && uint64(l.MaxDigestBytes) <= maxInt
}

func (p Parameters) valid(l Limits) bool {
	lanes := uint64(p.Parallelism)
	// Only canonical block layouts are supported; do not silently accept the
	// library's memory rounding or minimum-memory adjustment as another tuple.
	return lanes > 0 && p.Parallelism <= l.MaxParallelism &&
		p.Iterations > 0 && p.Iterations <= l.MaxIterations &&
		uint64(p.MemoryKiB) >= 8*lanes && uint64(p.MemoryKiB)%(4*lanes) == 0 &&
		p.MemoryKiB <= l.MaxMemoryKiB &&
		p.SaltBytes >= 8 && p.SaltBytes <= l.MaxSaltBytes &&
		p.DigestBytes >= 16 && p.DigestBytes <= l.MaxDigestBytes
}
