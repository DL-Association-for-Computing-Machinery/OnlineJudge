// Package emailmessage builds account emails for a trusted delivery adapter.
package emailmessage

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
)

// ErrInvalid does not include configuration, recipient or credential data.
var ErrInvalid = errors.New("invalid account email message input")

// Renderer uses an immutable operator-configured HTTPS origin, never an HTTP
// Host header or caller-provided return URL.
type Renderer struct{ host string }

// New checks the origin without opening connections or performing DNS lookups.
func New(origin string) (*Renderer, error) {
	if len(origin) > 300 {
		return nil, ErrInvalid
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.User != nil || origin != "https://"+u.Host || !validAuthority(u) {
		return nil, ErrInvalid
	}
	return &Renderer{host: u.Host}, nil
}

// Message contains sensitive fields intended for a trusted transport.
type Message struct {
	recipient identity.Email
	subject   string
	body      string
}

// Recipient returns the envelope destination.
func (m Message) Recipient() identity.Email { return m.recipient }

// Subject returns the fixed subject.
func (m Message) Subject() string { return m.subject }

// Text returns the plain text body, including its sensitive confirmation link.
func (m Message) Text() string { return m.body }

// Format redacts message data when directly formatted with fmt.
func (Message) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte("[redacted account email]"))
}

// MarshalJSON redacts the recipient and body.
func (Message) MarshalJSON() ([]byte, error) { return []byte(`"[redacted account email]"`), nil }

// Render builds a message using the recipient, purpose and token returned by
// the sender's eligibility check. It does not authorize delivery or consume a
// token. The browser route reads the fragment and sends the token only on POST.
func (r *Renderer) Render(recipient identity.Email, purpose emailtoken.Purpose, token emailtoken.Token) (Message, error) {
	if r == nil || r.host == "" || !recipient.Valid() {
		return Message{}, ErrInvalid
	}
	var path, subject, action string
	switch purpose {
	case emailtoken.Activation:
		path, subject, action = "/account/activate", "验证 OnlineJudge 邮箱", "验证邮箱并设置密码"
	case emailtoken.Recovery:
		path, subject, action = "/account/reset-password", "重置 OnlineJudge 密码", "设置新密码"
	default:
		return Message{}, ErrInvalid
	}
	value := token.LinkValue()
	if value == "" {
		return Message{}, ErrInvalid
	}
	link := url.URL{Scheme: "https", Host: r.host, Path: path, Fragment: "token=" + value}
	return Message{recipient: recipient, subject: subject,
		body: "请打开以下链接" + action + "：\n\n" + link.String() + "\n\n如果这不是你的操作，请忽略此邮件。\n"}, nil
}

func validAuthority(u *url.URL) bool {
	host := u.Hostname()
	authority := host
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Zone() != "" || ip.String() != host {
			return false
		}
		if ip.Is6() {
			authority = "[" + host + "]"
		}
	} else if !validDNSName(host) {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port || n == 443 {
			return false
		}
		authority += ":" + port
	}
	return u.Host == authority
}

func validDNSName(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	last := labels[len(labels)-1]
	// Avoid alternate browser IPv4 forms: a DNS suffix must start with a letter.
	if len(last) == 0 || last[0] < 'a' || last[0] > 'z' {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := range len(label) {
			c := label[i]
			switch {
			case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			default:
				return false
			}
		}
	}
	return true
}
