package identity_test

import (
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

func TestEmail(t *testing.T) {
	maxAddress := strings.Repeat("a", 64) + "@" + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)
	if len(maxAddress) != 254 {
		t.Fatal("invalid boundary fixture")
	}
	for _, raw := range []string{"User.Name+Tag@EXAMPLE.Invalid", "a!#$%&'*+-/=?^_`{|}~@example.invalid", strings.Repeat("a", 64) + "@example.invalid", "user@" + strings.Repeat("a", 63) + ".invalid", maxAddress} {
		t.Run(raw, func(t *testing.T) {
			e, err := identity.ParseEmail(raw)
			if err != nil || !e.Valid() || e.Original() != raw || e.Key() != strings.ToLower(raw) {
				t.Fatal("accepted email lost original address or whole-address case folding")
			}
			folded, err := identity.ParseEmail(strings.ToLower(raw))
			if err != nil || folded.Key() != e.Key() {
				t.Fatal("case variant did not resolve to the same key")
			}
		})
	}
	if (identity.Email{}).Valid() {
		t.Fatal("zero email accepted")
	}
	for _, raw := range []string{
		"", "a", "a@", "@example.invalid", "a@@example.invalid", "a..b@example.invalid", ".a@example.invalid", "a.@example.invalid",
		" User@example.invalid", "User@example.invalid ", "a\t@example.invalid", "a@example.invalid\r\nBcc: b@example.invalid", "a\x00@example.invalid",
		"User <a@example.invalid>", "<a@example.invalid>", "a@example.invalid (User)", "(User)a@example.invalid", "a@example.invalid,b@example.invalid",
		"\"a\"@example.invalid", "a@[127.0.0.1]", "a@exa_mple.invalid", "a@-example.invalid", "a@example-.invalid", "a@example..invalid", "a@example.invalid.",
		"用户@example.invalid", "a@例子.invalid", "ａ@example.invalid", strings.Repeat("a", 65) + "@example.invalid", "a@" + strings.Repeat("a", 64) + ".invalid",
		maxAddress + "d",
	} {
		t.Run(raw, func(t *testing.T) {
			e, err := identity.ParseEmail(raw)
			if err != identity.ErrInvalid || e != (identity.Email{}) {
				t.Fatal("unsupported email returned an accepted value or unstable error")
			}
		})
	}
	keys := map[string]bool{}
	for _, raw := range []string{"ab@example.invalid", "a.b@example.invalid", "ab+tag@example.invalid"} {
		e, err := identity.ParseEmail(raw)
		if err != nil || keys[e.Key()] {
			t.Fatal("dot or plus alias was merged")
		}
		keys[e.Key()] = true
	}
}
