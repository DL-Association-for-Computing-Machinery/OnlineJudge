package password

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func acceptanceRules(t *testing.T, minimum int, maxBytes uint32, secret string) *Acceptance {
	t.Helper()
	a, err := NewAcceptance(minimum, maxBytes, [][32]byte{sha256.Sum256([]byte(secret))})
	if err != nil {
		t.Fatal("valid explicit acceptance rules rejected")
	}
	return a
}

func TestAcceptanceLengthCountsCodePointsAndBoundsAllBytes(t *testing.T) {
	a := acceptanceRules(t, 3, 12, "synthetic blocked value")
	for _, tc := range []struct {
		name, secret string
		want         error
	}{
		{"ASCII minimum", "abc", nil}, {"CJK minimum", "三位字", nil}, {"four-byte minimum", "🙂🙂🙂", nil},
		{"insufficient multibyte", "éé", ErrNewPasswordLength}, {"empty", "", ErrNewPasswordLength},
		{"below minimum", "ab", ErrNewPasswordLength}, {"byte overflow", "🙂🙂🙂x", ErrNewPasswordLength},
		{"invalid UTF8", string([]byte{'a', 'b', 0xff}), ErrNewPasswordLength},
		{"combining code points", "e\u0301e", nil}, {"spaces remain input", "  a", nil}, {"NUL remains input", "\x00ab", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secret := []byte(tc.secret)
			before := bytes.Clone(secret)
			if err := a.Check(secret); !errors.Is(err, tc.want) || !bytes.Equal(secret, before) {
				t.Fatal("unexpected length result or modified input")
			}
		})
	}
	var zero Acceptance
	if zero.Check([]byte("abc")) != ErrAcceptance || (*Acceptance)(nil).Check(nil) != ErrAcceptance {
		t.Fatal("uninitialized acceptance rules accepted input")
	}
}

func TestAcceptanceLooksUpExactFullInputWithoutNormalization(t *testing.T) {
	a := acceptanceRules(t, 1, 128, "passwordpassword")
	if a.Check([]byte("passwordpassword")) != ErrWeakPassword {
		t.Fatal("exact weak password accepted")
	}
	for _, secret := range []string{" passwordpassword", "passwordpassword ", "Passwordpassword", "prefixpasswordpasswordsuffix"} {
		if a.Check([]byte(secret)) != nil {
			t.Fatal("blocklist matched transformed input or a substring")
		}
	}
	composed := acceptanceRules(t, 1, 128, "é")
	if composed.Check([]byte("é")) != ErrWeakPassword || composed.Check([]byte("e\u0301")) != nil {
		t.Fatal("acceptance unexpectedly normalized Unicode")
	}
}

func TestAcceptanceConstructorBudgetsCopyAndConcurrentReads(t *testing.T) {
	digest := sha256.Sum256([]byte("synthetic blocked"))
	for _, tc := range []struct {
		minimum int
		max     uint32
		blocked [][32]byte
	}{
		{0, 128, [][32]byte{digest}}, {65, 512, [][32]byte{digest}}, {3, 11, [][32]byte{digest}},
		{1, 0, [][32]byte{digest}}, {1, 128, nil}, {1, 128, [][32]byte{{}}}, {1, 128, [][32]byte{digest, digest}},
	} {
		if a, err := NewAcceptance(tc.minimum, tc.max, tc.blocked); a != nil || err != ErrAcceptance {
			t.Fatal("invalid acceptance budgets/list accepted")
		}
	}
	blocked := [][32]byte{digest}
	a, err := NewAcceptance(1, 128, blocked)
	if err != nil {
		t.Fatal("valid rule construction failed")
	}
	blocked[0] = sha256.Sum256([]byte("synthetic unblocked"))
	if a.Check([]byte("synthetic blocked")) != ErrWeakPassword || a.Check([]byte("synthetic unblocked")) != nil {
		t.Fatal("constructor retained mutable input list")
	}
	var group sync.WaitGroup
	for range 16 {
		group.Go(func() {
			for range 8 {
				if a.Check([]byte("synthetic blocked")) != ErrWeakPassword {
					t.Error("concurrent acceptance result changed")
				}
			}
		})
	}
	group.Wait()
	large := make([][32]byte, maxBlockedPasswords)
	for index := range large {
		large[index] = sha256.Sum256([]byte(strconv.Itoa(index)))
	}
	if a, err := NewAcceptance(64, 256, large); err != nil || a == nil {
		t.Fatal("maximum explicit policy/list budget rejected")
	}
	large = append(large, sha256.Sum256([]byte("additional synthetic entry")))
	if a, err := NewAcceptance(1, 128, large); a != nil || err != ErrAcceptance {
		t.Fatal("oversized blocklist accepted")
	}
}

func acceptanceDocument(t *testing.T, secret string) []byte {
	t.Helper()
	digest := sha256.Sum256([]byte(secret))
	body, err := json.Marshal(map[string]any{"version": 1, "sha256": []string{hex.EncodeToString(digest[:])}})
	if err != nil {
		t.Fatal("digest fixture encoding failed")
	}
	return body
}

func TestParseAcceptancePreservesExplicitRulesAndOrderIndependence(t *testing.T) {
	body := acceptanceDocument(t, "synthetic blocked")
	a, err := ParseAcceptance(body, 3, 128)
	if err != nil || a.Check([]byte("synthetic blocked")) != ErrWeakPassword || a.Check([]byte("ab")) != ErrNewPasswordLength || a.Check([]byte("abc")) != nil {
		t.Fatal("parsed explicit policy changed")
	}
	// Escaped names have the same decoded identity, including for duplicate
	// detection. Canonical spelling is required after standard JSON decoding.
	escaped := bytes.Replace(body, []byte(`"version"`), []byte(`"v\u0065rsion"`), 1)
	if _, err := ParseAcceptance(escaped, 3, 128); err != nil {
		t.Fatal("canonical decoded key rejected")
	}
	padded := append(bytes.Clone(body), bytes.Repeat([]byte{' '}, maxAcceptanceBytes-len(body))...)
	if _, err := ParseAcceptance(padded, 3, 128); err != nil {
		t.Fatal("exact document byte boundary rejected")
	}
}

func TestParseAcceptanceRejectsMalformedAmbiguousAndUnboundedInput(t *testing.T) {
	body := string(acceptanceDocument(t, "synthetic blocked"))
	digest := sha256.Sum256([]byte("synthetic blocked"))
	hexDigest := hex.EncodeToString(digest[:])
	for name, document := range map[string]string{
		"empty": "", "invalid JSON": "{", "null": "null", "root array": "[]", "root string": `"fixture"`,
		"missing version": `{"sha256":["` + hexDigest + `"]}`, "missing list": `{"version":1}`, "null list": `{"version":1,"sha256":null}`,
		"empty list": `{"version":1,"sha256":[]}`, "null digest": `{"version":1,"sha256":[null]}`, "nested digest": `{"version":1,"sha256":[[]]}`,
		"unknown":             `{"version":1,"sha256":["` + hexDigest + `"],"extra":1}`,
		"version float":       strings.Replace(body, `"version":1`, `"version":1.0`, 1),
		"version exponent":    strings.Replace(body, `"version":1`, `"version":1e0`, 1),
		"version string":      strings.Replace(body, `"version":1`, `"version":"1"`, 1),
		"version null":        strings.Replace(body, `"version":1`, `"version":null`, 1),
		"version unsupported": strings.Replace(body, `"version":1`, `"version":2`, 1),
		"version overflow":    strings.Replace(body, `"version":1`, `"version":99999999999999999999999999`, 1),
		"duplicate version":   strings.Replace(body, `"version":1`, `"version":1,"version":1`, 1),
		"escaped alias":       strings.Replace(body, `"version":1`, `"version":1,"v\u0065rsion":1`, 1),
		"case alias":          strings.Replace(body, `"version":1`, `"Version":1`, 1),
		"duplicate list":      `{"version":1,"sha256":["` + hexDigest + `"],"sha256":["` + hexDigest + `"]}`,
		"duplicate digest":    `{"version":1,"sha256":["` + hexDigest + `","` + hexDigest + `"]}`,
		"zero digest":         strings.Replace(body, hexDigest, strings.Repeat("0", 64), 1),
		"uppercase digest":    strings.Replace(body, hexDigest, strings.ToUpper(hexDigest), 1),
		"invalid hex":         strings.Replace(body, hexDigest, strings.Repeat("z", 64), 1),
		"short digest":        strings.Replace(body, hexDigest, hexDigest[:63], 1),
		"surrogate":           strings.Replace(body, hexDigest, `\ud800`, 1),
		"trailing object":     body + "{}", "trailing value": body + " true", "document overflow": body + strings.Repeat(" ", maxAcceptanceBytes),
	} {
		t.Run(name, func(t *testing.T) {
			a, err := ParseAcceptance([]byte(document), 3, 128)
			if a != nil || err != ErrAcceptance || err.Error() != "invalid new password acceptance configuration" {
				t.Fatal("invalid digest document was not safely rejected")
			}
		})
	}
	invalidUTF8 := bytes.Replace([]byte(body), []byte(hexDigest), []byte{0xff}, 1)
	if a, err := ParseAcceptance(invalidUTF8, 3, 128); a != nil || err != ErrAcceptance {
		t.Fatal("invalid UTF8 digest input accepted")
	}
	if a, err := ParseAcceptance([]byte(body), 0, 128); a != nil || err != ErrAcceptance {
		t.Fatal("parser defaulted missing minimum")
	}
}

func TestAcceptanceFormattingAndJSONNeverDumpRules(t *testing.T) {
	a := acceptanceRules(t, 3, 128, "synthetic blocked")
	for _, value := range []any{a, *a} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%f", "%x", "%X", "%b", "%o", "%e", "%c", "%U", "%t", "%020.3v"} {
			if fmt.Sprintf(format, value) != a.String() {
				t.Fatal("acceptance formatting exposed rules")
			}
		}
	}
	for _, value := range []any{a, *a, []any{a, *a}, map[string]any{"rules": a}} {
		body, err := json.Marshal(value)
		if err != nil || !bytes.Contains(body, []byte(a.String())) || bytes.Contains(body, []byte("minimum")) || bytes.Contains(body, []byte("blocked")) {
			t.Fatal("nested acceptance JSON exposed rules")
		}
	}
	if formatted := fmt.Sprintf("%+v", []*Acceptance{a}); formatted != "["+a.String()+"]" {
		t.Fatal("format container dumped acceptance internals")
	}
}

func TestAcceptanceFingerprintFreezesCompleteOrderIndependentRules(t *testing.T) {
	first := sha256.Sum256([]byte("synthetic blocked first"))
	second := sha256.Sum256([]byte("synthetic blocked second"))
	third := sha256.Sum256([]byte("synthetic blocked third"))
	blocked := [][32]byte{first, second}
	a, err := NewAcceptance(3, 128, blocked)
	if err != nil || a.Fingerprint() == ([32]byte{}) || a.MaxBytes() != 128 {
		t.Fatal("valid rules lack frozen fingerprint or byte budget")
	}
	want := a.Fingerprint()
	// Reordering and later mutation of the caller's slice change neither the
	// accepted set nor the fingerprint cached during construction.
	reversed, err := NewAcceptance(3, 128, [][32]byte{second, first})
	if err != nil || reversed.Fingerprint() != want {
		t.Fatal("digest order changed fingerprint")
	}
	blocked[0] = third
	if a.Fingerprint() != want || a.Check([]byte("synthetic blocked first")) != ErrWeakPassword || a.Check([]byte("synthetic blocked third")) != nil {
		t.Fatal("caller mutation changed frozen rules or fingerprint")
	}
	for _, tc := range []struct {
		minimum int
		maximum uint32
		set     [][32]byte
	}{
		{4, 128, [][32]byte{first, second}}, {3, 129, [][32]byte{first, second}},
		{3, 128, [][32]byte{first, third}}, {3, 128, [][32]byte{first}}, {3, 128, [][32]byte{first, second, third}},
	} {
		changed, err := NewAcceptance(tc.minimum, tc.maximum, tc.set)
		if err != nil || changed.Fingerprint() == want || changed.Fingerprint() == ([32]byte{}) {
			t.Fatal("changed explicit rule has unchanged or invalid fingerprint")
		}
	}
	var zero Acceptance
	for _, invalid := range []*Acceptance{nil, &zero} {
		if invalid.Fingerprint() != ([32]byte{}) || invalid.MaxBytes() != 0 || invalid.Check([]byte("synthetic value")) != ErrAcceptance {
			t.Fatal("invalid rules supplied a usable fingerprint or budget")
		}
	}
}
