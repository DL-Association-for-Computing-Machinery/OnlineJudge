package apiconfig

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPrivateDraftConfigurationExplicitAndBounded(t *testing.T) {
	absent, err := Parse([]byte(validDocument))
	if err != nil || absent.Runtime.PrivateDrafts != nil {
		t.Fatal("default enabled private drafts")
	}
	for _, kind := range []string{"valid", "reference-enabled", "review-enabled", "review-without-reference", "returns-enabled", "returns-without-access", "statement-enabled", "statement-without-access", "approvals-enabled", "approvals-without-access", "invalid-reference", "missing-root", "relative", "overlap", "internal-lock", "duplicate", "empty", "unbounded", "timeout", "write-timeout", "null", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			var wire map[string]any
			if json.Unmarshal([]byte(validDocument), &wire) != nil {
				t.Fatal("invalid base fixture")
			}
			value := map[string]any{"material_root": "/store", "bundle_directory": "/input/bundle", "source_lock_file": "/config/source-lock.json",
				"source_ids": []string{"library-checker:aplusb"}, "timeout": "1s", "max_in_flight": 1}
			wire["private_drafts"] = value
			switch kind {
			case "reference-enabled":
				value["reference_requests"] = true
			case "review-enabled":
				value["reference_requests"], value["review_intake"] = true, true
			case "review-without-reference":
				value["review_intake"] = true
			case "returns-enabled":
				value["review_access"], value["review_returns"] = true, true
			case "returns-without-access":
				value["review_returns"] = true
			case "statement-enabled":
				value["review_access"], value["statement_confirmations"] = true, true
			case "statement-without-access":
				value["statement_confirmations"] = true
			case "approvals-enabled":
				value["review_access"], value["review_approvals"] = true, true
			case "approvals-without-access":
				value["review_approvals"] = true
			case "invalid-reference":
				value["reference_requests"] = "true"
			case "missing-root":
				delete(value, "material_root")
			case "relative":
				value["bundle_directory"] = "input/bundle"
			case "overlap":
				value["material_root"] = "/input"
			case "internal-lock":
				value["source_lock_file"] = "/input/bundle/provenance/source-lock.json"
			case "duplicate":
				value["source_ids"] = []string{"a", "a"}
			case "empty":
				value["source_ids"] = []string{}
			case "unbounded":
				value["max_in_flight"] = 11
			case "timeout":
				value["timeout"] = "61s"
			case "write-timeout":
				value["timeout"] = "1m"
			case "null":
				wire["private_drafts"] = nil
			case "unknown":
				value["client_path"] = "/untrusted"
			}
			body, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			c, err := Parse(body)
			if kind == "valid" || kind == "reference-enabled" || kind == "review-enabled" || kind == "returns-enabled" || kind == "statement-enabled" || kind == "approvals-enabled" {
				if err != nil || c.Runtime.PrivateDrafts == nil || c.Runtime.PrivateDrafts.Timeout != time.Second ||
					c.Runtime.PrivateDrafts.ReferenceRequests != (kind == "reference-enabled" || kind == "review-enabled") ||
					c.Runtime.PrivateDrafts.ReviewIntake != (kind == "review-enabled") ||
					c.Runtime.PrivateDrafts.ReviewReturns != (kind == "returns-enabled") ||
					c.Runtime.PrivateDrafts.ReviewApprovals != (kind == "approvals-enabled") ||
					c.Runtime.PrivateDrafts.StatementConfirmations != (kind == "statement-enabled") {
					t.Fatal("explicit capability rejected")
				}
			} else if err == nil {
				t.Fatal("invalid capability accepted")
			}
		})
	}
}
