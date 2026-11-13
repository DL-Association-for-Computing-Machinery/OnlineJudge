package apiconfig

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEmailLimitsConfiguration(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(validDocument), &doc); err != nil {
		t.Fatal(err)
	}
	parse := func() (Config, error) {
		t.Helper()
		body, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		return Parse(body)
	}
	if c, err := parse(); err != nil || c.Runtime.EmailLimits != nil {
		t.Fatal("email limits must be optional")
	}
	names := []string{"request_global", "request_source", "request_mailbox", "confirm_global", "confirm_source", "confirm_token"}
	limits := make(map[string]any)
	doc["email_limits"] = limits
	for i, name := range names {
		limits[name] = map[string]any{"count": i + 1, "window": "1m"}
	}
	c, err := parse()
	if err != nil || c.Runtime.EmailLimits == nil {
		t.Fatal("complete email limits were rejected")
	}
	e := c.Runtime.EmailLimits
	for i, bucket := range []struct {
		count  int
		window time.Duration
	}{
		{e.RequestGlobal.Count, e.RequestGlobal.Window}, {e.RequestSource.Count, e.RequestSource.Window},
		{e.RequestMailbox.Count, e.RequestMailbox.Window}, {e.ConfirmGlobal.Count, e.ConfirmGlobal.Window},
		{e.ConfirmSource.Count, e.ConfirmSource.Window}, {e.ConfirmToken.Count, e.ConfirmToken.Window},
	} {
		if bucket.count != i+1 || bucket.window != time.Minute {
			t.Fatal("email limits were mixed or defaulted")
		}
	}
	for _, name := range names {
		old := limits[name]
		delete(limits, name)
		if _, err := parse(); err != ErrConfig {
			t.Fatalf("accepted missing %s", name)
		}
		limits[name] = old
	}
	limits["confirm_token"] = map[string]any{"count": 1, "window": "1500us"}
	if _, err := parse(); err != ErrConfig {
		t.Fatal("accepted a non-millisecond window")
	}
}
