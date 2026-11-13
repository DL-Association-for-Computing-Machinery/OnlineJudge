//go:build integration

package sessionprotocol_test

import (
	"crypto/sha256"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func testEmailLimits(t *testing.T, f *fixture) {
	for _, dimension := range []string{"global", "source", "target"} {
		t.Run(dimension, func(t *testing.T) {
			ns := namespace(t)
			prefix := "oj-email-limit:" + ns + ":"
			a, b := f.store(t, ns, 0, protocol.OrdinaryPolicy()), f.store(t, ns, 1, protocol.OrdinaryPolicy())
			secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
			bucket := protocol.LoginBucket{Count: 10, Window: 24 * time.Hour}
			limits := protocol.EmailLimits{RequestGlobal: bucket, RequestSource: bucket, RequestMailbox: bucket,
				ConfirmGlobal: bucket, ConfirmSource: bucket, ConfirmToken: bucket}
			source := netip.MustParseAddr("192.0.2.1")
			otherSource := netip.MustParseAddr("192.0.2.2")
			mailbox, err := identity.ParseEmail("unknown+tag@example.invalid")
			must(t, err)
			otherMailbox, err := identity.ParseEmail("other@example.invalid")
			must(t, err)
			digest := sha256.Sum256([]byte("synthetic first confirmation digest"))
			otherDigest := sha256.Sum256([]byte("synthetic second confirmation digest"))
			nextPurpose := emailtoken.Recovery
			switch dimension {
			case "global":
				limits.RequestGlobal.Count, limits.ConfirmGlobal.Count = 1, 1
			case "source":
				limits.RequestSource.Count, limits.ConfirmSource.Count = 1, 1
				otherSource = netip.MustParseAddr("::ffff:192.0.2.1")
			case "target":
				limits.RequestMailbox.Count, limits.ConfirmToken.Count = 1, 1
				otherMailbox, err = identity.ParseEmail("Unknown+Tag@EXAMPLE.INVALID")
				must(t, err)
				otherDigest, nextPurpose = digest, emailtoken.Activation
			}
			_, err = protocol.NewEmailAdmission(f.ctx, a, secrets, limits)
			wantError(t, err, protocol.ErrStore)
			must(t, protocol.InitializeEmailLimits(f.ctx, a, secrets, limits))
			first, err := protocol.NewEmailAdmission(f.ctx, a, secrets, limits)
			must(t, err)
			second, err := protocol.NewEmailAdmission(f.ctx, b, secrets, limits)
			must(t, err)
			mismatch := limits
			mismatch.ConfirmToken.Count++
			_, err = protocol.NewEmailAdmission(f.ctx, b, secrets, mismatch)
			wantError(t, err, protocol.ErrStore)
			otherSecrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
			_, err = protocol.NewEmailAdmission(f.ctx, b, otherSecrets, limits)
			wantError(t, err, protocol.ErrStore)
			must(t, first.AdmitRequest(f.ctx, source, mailbox))
			before := admissionSnapshot(t, f, prefix+"*")
			wantError(t, second.AdmitRequest(f.ctx, otherSource, otherMailbox), protocol.ErrEmailRate)
			if !reflect.DeepEqual(before, admissionSnapshot(t, f, prefix+"*")) {
				t.Fatal("request denial changed counters or deadlines")
			}
			must(t, second.AdmitConfirmation(f.ctx, source, emailtoken.Activation, digest))
			before = admissionSnapshot(t, f, prefix+"*")
			wantError(t, first.AdmitConfirmation(f.ctx, otherSource, nextPurpose, otherDigest), protocol.ErrEmailRate)
			if !reflect.DeepEqual(before, admissionSnapshot(t, f, prefix+"*")) {
				t.Fatal("confirmation denial changed counters or deadlines")
			}
			for key := range before {
				if strings.Contains(key, mailbox.Key()) || strings.Contains(key, source.String()) {
					t.Fatal("plaintext target or source appeared in a Redis key")
				}
			}
			must(t, f.control.Expire(f.ctx, prefix+"config", time.Hour).Err())
			wantError(t, second.AdmitRequest(f.ctx, source, mailbox), protocol.ErrStore)
			must(t, f.control.Del(f.ctx, prefix+"config").Err())
			wantError(t, second.AdmitRequest(f.ctx, source, mailbox), protocol.ErrStore)
		})
	}
}
