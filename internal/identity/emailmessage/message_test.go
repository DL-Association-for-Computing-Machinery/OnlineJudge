package emailmessage_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailmessage"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
)

func TestRenderAccountEmail(t *testing.T) {
	recipient, err := identity.ParseEmail("Mailbox+tag@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	token, err := emailtoken.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"https://oj.example.invalid", "https://localhost:8443", "https://127.0.0.1:8443", "https://[::1]:8443"} {
		r, err := emailmessage.New(origin)
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			purpose emailtoken.Purpose
			path    string
		}{
			{emailtoken.Activation, "/account/activate"},
			{emailtoken.Recovery, "/account/reset-password"},
		} {
			m, err := r.Render(recipient, tc.purpose, token)
			if err != nil {
				t.Fatal(err)
			}
			if m.Recipient() != recipient || m.Subject() == "" || strings.ContainsAny(m.Subject(), "\r\n") {
				t.Fatal("unexpected destination or subject")
			}
			lines := strings.Split(m.Text(), "\n")
			link, err := url.Parse(lines[2])
			if err != nil || "https://"+link.Host != origin || link.Scheme != "https" || link.Path != tc.path ||
				link.RequestURI() != tc.path || link.Fragment != "token="+token.LinkValue() ||
				strings.Count(m.Text(), token.LinkValue()) != 1 {
				t.Fatal("link did not keep the credential out of the request target")
			}
			encoded, err := json.Marshal(m)
			if err != nil || string(encoded) != `"[redacted account email]"` || fmt.Sprintf("%#v", m) != "[redacted account email]" ||
				fmt.Sprintf("%s", &m) != "[redacted account email]" {
				t.Fatal("mail diagnostics exposed sensitive fields")
			}
		}
		if _, err := r.Render(recipient, "other", token); !errors.Is(err, emailmessage.ErrInvalid) {
			t.Fatal("unexpected purpose was accepted")
		}
		if _, err := r.Render(recipient, emailtoken.Activation, emailtoken.Token{}); !errors.Is(err, emailmessage.ErrInvalid) {
			t.Fatal("zero token was accepted")
		}
		if _, err := r.Render(identity.Email{}, emailtoken.Activation, token); !errors.Is(err, emailmessage.ErrInvalid) {
			t.Fatal("zero recipient was accepted")
		}
	}
	for _, r := range []*emailmessage.Renderer{nil, {}} {
		if _, err := r.Render(recipient, emailtoken.Activation, token); !errors.Is(err, emailmessage.ErrInvalid) {
			t.Fatal("unconfigured renderer was accepted")
		}
	}
}

func TestRejectInvalidOrigin(t *testing.T) {
	for _, origin := range []string{
		"", "http://oj.example.invalid", "//oj.example.invalid", "https://user@oj.example.invalid",
		"https://oj.example.invalid/", "https://oj.example.invalid/a", "https://oj.example.invalid?",
		"https://oj.example.invalid#", "https://oj.example.invalid:443", "https://oj.example.invalid:0443",
		"https://oj.example.invalid:", "https://oj.example.invalid:0", "https://oj.example.invalid:65536",
		"https://oj.example.invalid\n", "https://OJ.example.invalid", "https://oj..invalid",
		"https://127.1", "https://0x7f000001", "https://[fe80::1%25eth0]", "https://[oj.example.invalid]",
		"https://[0:0:0:0:0:0:0:1]", "https://oj.example.invalid.", "https://" + strings.Repeat("a", 64) + ".invalid",
		"https://" + strings.Repeat("a.", 150) + "invalid",
	} {
		if r, err := emailmessage.New(origin); r != nil || err != emailmessage.ErrInvalid {
			t.Fatalf("unexpected acceptance for configured origin %q", origin)
		}
	}
}
