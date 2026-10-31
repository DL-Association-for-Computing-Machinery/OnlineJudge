package password

import (
	"math"
	"testing"
)

func TestMalformedRecordsRejectBeforeDerivation(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Record)
	}{
		{"format-zero", func(r *Record) { r.FormatVersion = 0 }},
		{"format-new", func(r *Record) { r.FormatVersion++ }},
		{"algorithm", func(r *Record) { r.Algorithm = "argon2i" }},
		{"argon-version", func(r *Record) { r.ArgonVersion++ }},
		{"zero-passes", func(r *Record) { r.Parameters.Iterations = 0 }},
		{"excess-passes", func(r *Record) { r.Parameters.Iterations = math.MaxUint32 }},
		{"zero-lanes", func(r *Record) { r.Parameters.Parallelism = 0 }},
		{"excess-lanes", func(r *Record) { r.Parameters.Parallelism = math.MaxUint8 }},
		{"small-memory", func(r *Record) { r.Parameters.MemoryKiB = 4 }},
		{"rounding-memory", func(r *Record) { r.Parameters.MemoryKiB = 63 }},
		{"excess-memory", func(r *Record) { r.Parameters.MemoryKiB = math.MaxUint32 }},
		{"unlisted-cost", func(r *Record) { r.Parameters.MemoryKiB = 128 }},
		{"small-salt-policy", func(r *Record) { r.Parameters.SaltBytes = 7 }},
		{"excess-salt-policy", func(r *Record) { r.Parameters.SaltBytes = math.MaxUint32 }},
		{"small-digest-policy", func(r *Record) { r.Parameters.DigestBytes = 15 }},
		{"excess-digest-policy", func(r *Record) { r.Parameters.DigestBytes = math.MaxUint32 }},
		{"short-salt", func(r *Record) { r.Salt = r.Salt[:len(r.Salt)-1] }},
		{"long-salt", func(r *Record) { r.Salt = append(r.Salt, 0) }},
		{"nil-salt", func(r *Record) { r.Salt = nil }},
		{"short-digest", func(r *Record) { r.Digest = r.Digest[:len(r.Digest)-1] }},
		{"long-digest", func(r *Record) { r.Digest = append(r.Digest, 0) }},
		{"nil-digest", func(r *Record) { r.Digest = nil }},
		{"unlisted-salt-length", func(r *Record) { r.Parameters.SaltBytes++; r.Salt = append(r.Salt, 0) }},
		{"unlisted-digest-length", func(r *Record) { r.Parameters.DigestBytes++; r.Digest = append(r.Digest, 0) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := testHasher(t)
			h.derive = func([]byte, []byte, Parameters) []byte { t.Fatal("rejected record derived"); return nil }
			r := vectorRecord(t)
			tc.edit(&r)
			got, err := h.Verify([]byte("password"), r)
			if err != ErrRecord || got != (Verification{}) {
				t.Fatalf("malformed record: result=%v error=%v", got, err)
			}
		})
	}
}

func TestInvalidPolicyRejectsAtConstruction(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Policy, *Limits)
	}{
		{"zero-policy", func(p *Policy, _ *Limits) { *p = Policy{} }},
		{"zero-limits", func(_ *Policy, l *Limits) { *l = Limits{} }},
		{"zero-memory-limit", func(_ *Policy, l *Limits) { l.MaxMemoryKiB = 0 }},
		{"zero-pass-limit", func(_ *Policy, l *Limits) { l.MaxIterations = 0 }},
		{"zero-lane-limit", func(_ *Policy, l *Limits) { l.MaxParallelism = 0 }},
		{"zero-password-limit", func(_ *Policy, l *Limits) { l.MaxPasswordBytes = 0 }},
		{"small-salt-limit", func(_ *Policy, l *Limits) { l.MaxSaltBytes = 7 }},
		{"small-digest-limit", func(_ *Policy, l *Limits) { l.MaxDigestBytes = 15 }},
		{"zero-passes", func(p *Policy, _ *Limits) { p.Current.Iterations = 0 }},
		{"zero-lanes", func(p *Policy, _ *Limits) { p.Current.Parallelism = 0 }},
		{"small-memory", func(p *Policy, _ *Limits) { p.Current.MemoryKiB = 8 }},
		{"rounding-memory", func(p *Policy, _ *Limits) { p.Current.MemoryKiB = 127 }},
		{"salt-minimum", func(p *Policy, _ *Limits) { p.Current.SaltBytes = 7 }},
		{"digest-minimum", func(p *Policy, _ *Limits) { p.Current.DigestBytes = 15 }},
		{"memory-limit", func(p *Policy, _ *Limits) { p.Current.MemoryKiB = 264 }},
		{"passes-limit", func(p *Policy, _ *Limits) { p.Current.Iterations = 5 }},
		{"lanes-limit", func(p *Policy, _ *Limits) { p.Current.Parallelism = 5 }},
		{"salt-limit", func(p *Policy, _ *Limits) { p.Current.SaltBytes = 33 }},
		{"digest-limit", func(p *Policy, _ *Limits) { p.Current.DigestBytes = 65 }},
		{"invalid-legacy", func(p *Policy, _ *Limits) { p.Legacy[0].Iterations = 0 }},
		{"legacy-over-limit", func(p *Policy, _ *Limits) { p.Legacy[0].Iterations = 5 }},
		{"duplicate-legacy", func(p *Policy, _ *Limits) { p.Legacy = append(p.Legacy, p.Legacy[0]) }},
		{"current-in-legacy", func(p *Policy, _ *Limits) { p.Legacy = append(p.Legacy, p.Current) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, l := testPolicy()
			tc.edit(&p, &l)
			if h, err := New(p, l); err != ErrPolicy || h != nil {
				t.Fatal("invalid configuration produced a hasher")
			}
		})
	}
}

func TestExactPolicyCeilingsAndStructuralMinima(t *testing.T) {
	p := Parameters{MemoryKiB: 8, Iterations: 1, Parallelism: 1, SaltBytes: 8, DigestBytes: 16}
	l := Limits{MaxMemoryKiB: 8, MaxIterations: 1, MaxParallelism: 1,
		MaxPasswordBytes: 1, MaxSaltBytes: 8, MaxDigestBytes: 16}
	h, err := New(Policy{Current: p}, l)
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.Create([]byte{0})
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.Verify([]byte{0}, r)
	if err != nil || !got.Match || got.NeedsUpgrade {
		t.Fatalf("exact minima: result=%v error=%v", got, err)
	}
}
