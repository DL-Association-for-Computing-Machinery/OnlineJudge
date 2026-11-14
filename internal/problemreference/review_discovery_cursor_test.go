package problemreference

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func TestReviewCursorConcealsHistoricalIDAndBindsActor(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	box, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := redisstore.ParseCredential(strings.Repeat("A", 43))
	if err != nil {
		t.Fatal(err)
	}
	other, err := redisstore.ParseCredential(strings.Repeat("B", 42) + "A")
	if err != nil {
		t.Fatal(err)
	}
	actor := identity.ProblemActor{SubjectID: strings.Repeat("1", 32), SessionVersion: identity.SessionVersion{
		RecoveryID: strings.Repeat("2", 32), AuthGeneration: 3,
	}}
	position := ReviewDiscoveryPosition{ID: strings.Repeat("4", 32), RoleRevision: 5}
	raw := sealReviewCursor(box, position, credential, actor)
	got, err := openReviewCursor(box, raw, credential, actor)
	if err != nil || got != position || len(raw) != 91 || raw == sealReviewCursor(box, position, credential, actor) {
		t.Fatalf("cursor failed its canonical round trip or reused a nonce: %v", err)
	}
	moved := actor
	moved.AuthGeneration++
	substitute := "A"
	if raw[50] == 'A' {
		substitute = "B"
	}
	for _, tc := range []struct {
		raw   string
		token redisstore.Credential
		actor identity.ProblemActor
	}{
		{raw, other, actor}, {raw, credential, moved},
		{raw[:50] + substitute + raw[51:], credential, actor},
		{raw + "=", credential, actor},
	} {
		if got, err := openReviewCursor(box, tc.raw, tc.token, tc.actor); got != (ReviewDiscoveryPosition{}) || !errors.Is(err, ErrReviewCursor) {
			t.Fatal("accepted a foreign, stale, or modified cursor")
		}
	}
}
