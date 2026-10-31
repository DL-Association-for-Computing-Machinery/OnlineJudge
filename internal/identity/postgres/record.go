package postgres

import (
	"math"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
)

func writeParams(id string, want identity.Revision, r password.Record) identitydb.RehashCASParams {
	// checkWrite matched approved metadata and constructor bounded both lengths
	// before narrowing the fixed format/version and length fields to SQL int32.
	p := r.Parameters
	return identitydb.RehashCASParams{
		ID: id, ExpectedCredentialVersion: want.CredentialVersion, ExpectedAuthGeneration: want.AuthGeneration,
		FormatVersion: int32(r.FormatVersion), Algorithm: r.Algorithm, ArgonVersion: int32(r.ArgonVersion),
		MemoryKib: int64(p.MemoryKiB), Iterations: int64(p.Iterations), Parallelism: int16(p.Parallelism),
		SaltBytes: int32(p.SaltBytes), DigestBytes: int32(p.DigestBytes), Salt: r.Salt, Digest: r.Digest,
	}
}

func (s *Store) snapshot(row identitydb.IdentityCredential) (identity.Snapshot, error) {
	if !identity.ValidID(row.ID) || row.CredentialVersion <= 0 || row.AuthGeneration <= 0 ||
		row.FormatVersion < 0 || row.ArgonVersion < 0 ||
		row.MemoryKib < 0 || row.MemoryKib > math.MaxUint32 || row.Iterations < 0 || row.Iterations > math.MaxUint32 ||
		row.Parallelism < 0 || row.Parallelism > math.MaxUint8 || row.SaltBytes < 0 || row.DigestBytes < 0 {
		return identity.Snapshot{}, password.ErrRecord
	}
	r := password.Record{
		FormatVersion: uint32(row.FormatVersion), Algorithm: row.Algorithm, ArgonVersion: uint32(row.ArgonVersion),
		Parameters: password.Parameters{
			MemoryKiB: uint32(row.MemoryKib), Iterations: uint32(row.Iterations), Parallelism: uint8(row.Parallelism),
			SaltBytes: uint32(row.SaltBytes), DigestBytes: uint32(row.DigestBytes),
		}, Salt: row.Salt, Digest: row.Digest,
	}
	if _, err := s.hasher.CheckRecord(r); err != nil {
		return identity.Snapshot{}, err
	}
	return identity.Snapshot{State: identity.State{ID: row.ID, Enabled: row.Enabled,
		Revision: identity.Revision{CredentialVersion: row.CredentialVersion, AuthGeneration: row.AuthGeneration}}, Record: r}, nil
}
