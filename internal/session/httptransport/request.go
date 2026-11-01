package httptransport

import (
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func (h *handler) sameOrigin(r *http.Request) bool {
	if h.protection.Check(r) != nil {
		return false
	}
	origins, originPresent := r.Header["Origin"]
	sites, sitePresent := r.Header["Sec-Fetch-Site"]
	if originPresent && (len(origins) != 1 || origins[0] != h.config.Origin) {
		return false
	}
	if sitePresent && (len(sites) != 1 || sites[0] != "same-origin") {
		return false
	}
	// Unlike the standard compatibility default, writes need positive evidence.
	return originPresent || sitePresent
}

func requestCredential(r *http.Request) (redisstore.Credential, error) {
	value, present, err := cookieValue(r, CookieName)
	if err != nil || !present {
		return redisstore.Credential{}, redisstore.ErrInvalid
	}
	return redisstore.ParseCredential(value)
}

func cookieValue(r *http.Request, name string) (string, bool, error) {
	var value string
	count := 0
	for _, line := range r.Header.Values("Cookie") {
		cookies, err := http.ParseCookie(line)
		if err != nil {
			return "", false, redisstore.ErrInvalid
		}
		for _, cookie := range cookies {
			if cookie.Name == name {
				count++
				value = cookie.Value
			}
		}
	}
	if count > 1 {
		return "", false, redisstore.ErrInvalid
	}
	return value, count == 1, nil
}
