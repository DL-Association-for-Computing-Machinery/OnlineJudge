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
	doc["email_confirmation"] = map[string]any{"minimum_characters": 15, "blocklist_file": "/config/weak-passwords.json"}
	if _, err := parse(); err != ErrConfig {
		t.Fatal("confirmation without email limits was accepted")
	}
	names := []string{"request_global", "request_source", "request_mailbox", "confirm_global", "confirm_source", "confirm_token"}
	limits := make(map[string]any)
	doc["email_limits"] = limits
	for i, name := range names {
		limits[name] = map[string]any{"count": i + 1, "window": "1m"}
	}
	c, err := parse()
	if err != nil || c.Runtime.EmailLimits == nil || c.EmailMinimumCharacters != 15 || c.EmailBlocklistFile != "/config/weak-passwords.json" {
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
	budget := make(map[string]any)
	for name, value := range doc["database"].(map[string]any) {
		budget[name] = value
	}
	budget["database"], budget["auth"] = "seal_budget", "env"
	intake := map[string]any{"capacity": 4, "workers": 2, "max_age": "5s",
		"payload_key_file": "/config/mail-keys.json", "budget_database": budget}
	doc["email_intake"] = intake
	c, err = parse()
	if err != nil || c.Runtime.EmailIntake == nil || c.EmailBudgetAuth != "env" ||
		c.Runtime.EmailIntake.BudgetDatabase.Database != "seal_budget" ||
		c.Runtime.EmailIntake.Limits.Capacity != 4 || c.Runtime.EmailIntake.Limits.Workers != 2 ||
		c.Runtime.EmailIntake.Limits.MaxAge != 5*time.Second {
		t.Fatal("explicit email intake settings were not preserved")
	}
	confirmation := doc["email_confirmation"]
	delete(doc, "email_confirmation")
	if _, err := parse(); err != ErrConfig {
		t.Fatal("accepted intake without confirmation")
	}
	doc["email_confirmation"] = confirmation
	for field, bad := range map[string]any{"capacity": 1, "workers": 0, "max_age": "61s", "payload_key_file": "relative", "budget_database": nil} {
		old := intake[field]
		intake[field] = bad
		if _, err := parse(); err != ErrConfig {
			t.Fatalf("accepted invalid intake field %s", field)
		}
		intake[field] = old
	}
	budget["database"] = doc["database"].(map[string]any)["database"]
	if _, err := parse(); err != ErrConfig {
		t.Fatal("accepted the application database as the budget database")
	}
	delete(doc, "email_intake")
	limits["confirm_token"] = map[string]any{"count": 1, "window": "1500us"}
	if _, err := parse(); err != ErrConfig {
		t.Fatal("accepted a non-millisecond window")
	}
}
