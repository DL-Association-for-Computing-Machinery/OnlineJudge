package problemreference

import (
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strconv"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const reviewCursorPlainBytes = 40

func reviewCursorActor(c redisstore.Credential, a identity.ProblemActor) []byte {
	binding := sha256.Sum256([]byte(c.Value()))
	return append(binding[:], a.SubjectID+":"+a.RecoveryID+":"+strconv.FormatInt(a.AuthGeneration, 10)...)
}

func sealReviewCursor(box cipher.AEAD, p ReviewDiscoveryPosition, c redisstore.Credential, a identity.ProblemActor) string {
	if p.ID == "" {
		return ""
	}
	cleartext := binary.BigEndian.AppendUint64([]byte(p.ID), uint64(p.RoleRevision))
	return base64.RawURLEncoding.EncodeToString(box.Seal(nil, nil, cleartext, reviewCursorActor(c, a)))
}

func openReviewCursor(box cipher.AEAD, raw string, c redisstore.Credential, a identity.ProblemActor) (ReviewDiscoveryPosition, error) {
	var zero ReviewDiscoveryPosition
	if raw == "" {
		return zero, nil
	}
	if len(raw) != base64.RawURLEncoding.EncodedLen(reviewCursorPlainBytes+box.Overhead()) {
		return zero, ErrReviewCursor
	}
	encoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(encoded) != raw {
		return zero, ErrReviewCursor
	}
	cleartext, err := box.Open(nil, nil, encoded, reviewCursorActor(c, a))
	if err != nil || len(cleartext) != reviewCursorPlainBytes {
		return zero, ErrReviewCursor
	}
	p := ReviewDiscoveryPosition{ID: string(cleartext[:32]), RoleRevision: int64(binary.BigEndian.Uint64(cleartext[32:]))}
	if !identity.ValidID(p.ID) || p.RoleRevision < 1 {
		return zero, ErrReviewCursor
	}
	return p, nil
}
