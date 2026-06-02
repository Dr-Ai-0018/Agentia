package main

import (
	"strings"
	"testing"
)

func TestDecodeDraftRejectsDuplicateKeys(t *testing.T) {
	raw := `{
		"event_anchor":"a",
		"old_read_to_drop":"b",
		"new_read_to_keep":"c",
		"carry_forward_rule":"d",
		"why_it_matters":"e",
		"scope_boundary":"f",
		"confidence":88,
		"promote_or_decay":"promote_permanent",
		"permanent_decision":"x",
		"permanent_decision":"y"
	}`

	_, err := decodeDraft(raw)
	if err == nil {
		t.Fatal("expected duplicate key error")
	}
	if !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("expected duplicate key error, got %v", err)
	}
}

func TestDuplicateJSONObjectKeysNested(t *testing.T) {
	raw := `{"outer":{"a":1,"a":2},"b":[{"c":1,"c":2}]}`
	issues := duplicateJSONObjectKeys(raw)
	if len(issues) != 2 {
		t.Fatalf("expected 2 duplicate key issues, got %d: %v", len(issues), issues)
	}
}

func TestDecodeRoutingDecision(t *testing.T) {
	raw := `{
		"target_layer":"long",
		"action":"promote",
		"reason_codes":["multi_round_decision_impact"],
		"review_after":"168h",
		"expires_after":"504h"
	}`

	decision, err := decodeRoutingDecision(raw)
	if err != nil {
		t.Fatalf("decodeRoutingDecision returned error: %v", err)
	}
	if decision.TargetLayer != "long" {
		t.Fatalf("unexpected target layer: %s", decision.TargetLayer)
	}
	if decision.Action != "promote" {
		t.Fatalf("unexpected action: %s", decision.Action)
	}
	if len(decision.ReasonCodes) != 1 || decision.ReasonCodes[0] != "multi_round_decision_impact" {
		t.Fatalf("unexpected reason codes: %#v", decision.ReasonCodes)
	}
}

func TestDecodeDraftRejectsUnexpectedTopLevelKey(t *testing.T) {
	raw := `{
		"event_anchor":"a",
		"old_read_to_drop":"b",
		"new_read_to_keep":"c",
		"carry_forward_rule":"d",
		"why_it_matters":"e",
		"scope_boundary":"f",
		"confidence":88,
		"promote_or_decay":"promote_permanent",
		"permanent_decision":"x",
		"permanent":true
	}`

	_, err := decodeDraft(raw)
	if err == nil {
		t.Fatal("expected unexpected top-level key error")
	}
	if !strings.Contains(err.Error(), "unexpected top-level key: permanent") {
		t.Fatalf("expected unexpected top-level key error, got %v", err)
	}
}

func TestDecodeActionDecisionRejectsInvalidEnum(t *testing.T) {
	raw := `{
		"action":"invent",
		"reason_codes":["bad"],
		"needs_review":false,
		"review_after":null,
		"expires_after":null
	}`

	_, err := decodeActionDecision(raw)
	if err == nil {
		t.Fatal("expected invalid enum error")
	}
	if !strings.Contains(err.Error(), "action has invalid enum value") {
		t.Fatalf("expected invalid enum error, got %v", err)
	}
}
