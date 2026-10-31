package password

import (
	"bytes"
	"encoding/hex"
	"errors"
	"slices"
	"sync"
	"testing"

	"golang.org/x/crypto/argon2"
)

// These tiny costs exercise the contract, not production password security.
func testPolicy() (Policy, Limits) {
	return Policy{
			Current: Parameters{MemoryKiB: 128, Iterations: 2, Parallelism: 2, SaltBytes: 16, DigestBytes: 32},
			Legacy:  []Parameters{{MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltBytes: 8, DigestBytes: 24}},
		}, Limits{MaxMemoryKiB: 256, MaxIterations: 4, MaxParallelism: 4,
			MaxPasswordBytes: 64, MaxSaltBytes: 32, MaxDigestBytes: 64}
}

func testHasher(t *testing.T) *Hasher {
	t.Helper()
	p, l := testPolicy()
	h, err := New(p, l)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func vectorRecord(t *testing.T) Record {
	t.Helper()
	// Upstream CLI-generated Argon2id vector, x/crypto v0.58.0:
	// https://github.com/golang/crypto/blob/v0.58.0/argon2/argon2_test.go
	digest, err := hex.DecodeString("655ad15eac652dc59f7170a7332bf49b8469be1fdb9c28bb")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := testPolicy()
	return Record{FormatVersion: 1, Algorithm: "argon2id", ArgonVersion: 0x13,
		Parameters: p.Legacy[0], Salt: []byte("somesalt"), Digest: digest}
}

func TestKnownVectorAndMigrationHint(t *testing.T) {
	h := testHasher(t)
	r := vectorRecord(t)
	got, err := h.Verify([]byte("password"), r)
	if err != nil || !got.Match || !got.NeedsUpgrade {
		t.Fatalf("known legacy vector: result=%v error=%v", got, err)
	}
	got, err = h.Verify([]byte("wrong-password"), r)
	if err != nil || got != (Verification{}) {
		t.Fatalf("wrong legacy password: result=%v error=%v", got, err)
	}
	_, l := testPolicy()
	writer, err := New(Policy{Current: r.Parameters}, l)
	if err != nil {
		t.Fatal(err)
	}
	writer.salt = func(dst []byte) error { copy(dst, r.Salt); return nil }
	written, err := writer.Create([]byte("password"))
	if err != nil || !bytes.Equal(written.Digest, r.Digest) || written.ArgonVersion != argon2.Version {
		t.Fatalf("known vector creation failed: %v", err)
	}
	got, err = writer.Verify([]byte("password"), written)
	if err != nil || !got.Match || got.NeedsUpgrade {
		t.Fatalf("current vector: result=%v error=%v", got, err)
	}
}

func TestCreateUsesIndependentRandomSalts(t *testing.T) {
	h := testHasher(t)
	password := []byte("  密码\x00with trailing space ")
	before := slices.Clone(password)
	first, err := h.Create(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.Create(password)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first.Salt, second.Salt) || bytes.Equal(first.Digest, second.Digest) {
		t.Fatal("fresh records reused verification material")
	}
	for _, record := range []Record{first, second} {
		salt, digest := slices.Clone(record.Salt), slices.Clone(record.Digest)
		got, err := h.Verify(password, record)
		if err != nil || !got.Match || got.NeedsUpgrade {
			t.Fatalf("fresh verification: result=%v error=%v", got, err)
		}
		if !bytes.Equal(record.Salt, salt) || !bytes.Equal(record.Digest, digest) {
			t.Fatal("verification mutated the caller's record")
		}
		for _, changed := range [][]byte{bytes.TrimSpace(password), []byte("密码\x00with trailing space "), []byte("  密码")} {
			got, err = h.Verify(changed, record)
			if err != nil || got != (Verification{}) {
				t.Fatalf("altered password accepted: result=%v error=%v", got, err)
			}
		}
	}
	if !bytes.Equal(password, before) {
		t.Fatal("password input was modified")
	}
}

func TestPasswordByteBoundaryBeforeDerivation(t *testing.T) {
	h := testHasher(t)
	password := bytes.Repeat([]byte{0xff}, 64)
	record, err := h.Create(password)
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.Verify(password, record)
	if err != nil || !got.Match {
		t.Fatalf("exact byte limit: result=%v error=%v", got, err)
	}
	h.derive = func([]byte, []byte, Parameters) []byte { t.Fatal("invalid input derived"); return nil }
	h.salt = func([]byte) error { t.Fatal("invalid input used randomness"); return nil }
	for _, input := range [][]byte{nil, {}, bytes.Repeat([]byte{0xff}, 65), bytes.Repeat([]byte("中"), 22)} {
		if _, err := h.Create(input); !errors.Is(err, ErrPassword) {
			t.Fatalf("create invalid length: %v", err)
		}
		if got, err := h.Verify(input, record); !errors.Is(err, ErrPassword) || got != (Verification{}) {
			t.Fatalf("verify invalid length: result=%v error=%v", got, err)
		}
	}
}

func TestRandomFailureDoesNotProduceRecordOrDerive(t *testing.T) {
	h := testHasher(t)
	h.salt = func(dst []byte) error { dst[0] = 42; return errors.New("private diagnostic") }
	h.derive = func([]byte, []byte, Parameters) []byte { t.Fatal("random failure derived"); return nil }
	r, err := h.Create([]byte("password"))
	if err != ErrRandom || r.Algorithm != "" || r.Salt != nil || r.Digest != nil || r.Parameters != (Parameters{}) {
		t.Fatal("random failure leaked a partial record or diagnostic")
	}
}

func TestConstructedPolicyIsIndependentOfCaller(t *testing.T) {
	p, l := testPolicy()
	h, err := New(p, l)
	if err != nil {
		t.Fatal(err)
	}
	current := p.Current
	p.Current.Iterations = 0
	p.Legacy[0].MemoryKiB = 0
	l.MaxPasswordBytes = 0
	r, err := h.Create([]byte("password"))
	if err != nil || r.Parameters != current {
		t.Fatalf("caller mutation changed current policy: %v", err)
	}
	got, err := h.Verify([]byte("password"), vectorRecord(t))
	if err != nil || !got.Match || !got.NeedsUpgrade {
		t.Fatalf("caller mutation changed legacy policy: result=%v error=%v", got, err)
	}
}

func TestZeroHasherFailsClosed(t *testing.T) {
	for _, h := range []*Hasher{nil, {}} {
		if _, err := h.Create([]byte("password")); err != ErrPolicy {
			t.Fatalf("zero create: %v", err)
		}
		if got, err := h.Verify([]byte("password"), Record{}); err != ErrPolicy || got != (Verification{}) {
			t.Fatalf("zero verify: result=%v error=%v", got, err)
		}
	}
}

func TestConcurrentCallsUseImmutablePolicy(t *testing.T) {
	h := testHasher(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			password := []byte("parallel synthetic password")
			r, err := h.Create(password)
			if err != nil {
				t.Error(err)
				return
			}
			got, err := h.Verify(password, r)
			if err != nil || !got.Match || got.NeedsUpgrade {
				t.Errorf("parallel verification: result=%v error=%v", got, err)
			}
		})
	}
	wg.Wait()
}
