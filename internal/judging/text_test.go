package judging_test

import (
	"errors"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"strings"
	"testing"
)

func TestTextRules(t *testing.T) {
	for _, tc := range []struct {
		want, got     string
		sensitive, ok bool
	}{
		{"YES 1 2", " yes\t1\n2\r\v\f", false, true},
		{"YES", "yes", true, false}, {"1", "1.0", false, false},
		{"1", "001", false, false}, {"a b", "a", false, false}, {"a", "a b", false, false},
		{"", " \n", false, true}, {"", "x", false, false}, {"Ä", "ä", false, false},
		{"a\u00a0b", "a b", false, false}, {"中文", "中文", false, true},
		{"yes", string([]byte{0xff}), false, false},
	} {
		ok, err := judging.CompareText([]byte(tc.want), []byte(tc.got), tc.sensitive)
		if err != nil || ok != tc.ok {
			t.Fatalf("%q vs %q: %v %v", tc.want, tc.got, ok, err)
		}
	}
}

func TestTextBudgetsAndReferencePrecedence(t *testing.T) {
	for _, tc := range []struct {
		reference, output []byte
		err               error
	}{
		{[]byte{0xff}, []byte("wrong"), judging.ErrReference},
		{[]byte("valid " + strings.Repeat("x", 65537)), []byte("wrong"), judging.ErrReference},
		{[]byte("valid"), []byte(strings.Repeat("x", judging.TextBudget+1)), judging.ErrOutput},
		{[]byte("valid"), []byte(strings.Repeat("x", 65537)), judging.ErrOutput},
	} {
		_, err := judging.CompareText(tc.reference, tc.output, false)
		if !errors.Is(err, tc.err) {
			t.Fatalf("wanted %v got %v", tc.err, err)
		}
	}
}
