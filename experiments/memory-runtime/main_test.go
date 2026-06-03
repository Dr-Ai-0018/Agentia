package main

import (
	"strings"
	"testing"
	"time"

	"ai-arena/internal/memory"
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

func TestDecodeActionDecisionRejectsInvalidDuration(t *testing.T) {
	raw := `{
		"action":"update",
		"reason_codes":["x"],
		"needs_review":false,
		"review_after":null,
		"expires_after":"never"
	}`

	_, err := decodeActionDecision(raw)
	if err == nil {
		t.Fatal("expected invalid duration error")
	}
	if !strings.Contains(err.Error(), "expires_after must be a valid Go duration or null") {
		t.Fatalf("expected invalid duration error, got %v", err)
	}
}

func TestDecodeAdjudicationDecision(t *testing.T) {
	raw := `{
		"accepted": true,
		"reject_reason": "",
		"issues": [],
		"target_layer": "short",
		"action": "create",
		"needs_review": true,
		"review_after": "8h",
		"expires_after": "24h",
		"reason_codes": ["grounded_in_incident"]
	}`

	decision, err := decodeAdjudicationDecision(raw)
	if err != nil {
		t.Fatalf("decodeAdjudicationDecision returned error: %v", err)
	}
	if !decision.Accepted || decision.TargetLayer != "short" || decision.Action != "create" {
		t.Fatalf("unexpected adjudication decision: %#v", decision)
	}
}

func TestShouldSkipByPolicy(t *testing.T) {
	if !shouldSkipByPolicy(memoryDecision("retain", "instant")) {
		t.Fatal("expected retain instant to skip new memory")
	}
	if shouldSkipByPolicy(memoryDecision("update", "short")) {
		t.Fatal("did not expect update short to skip new memory")
	}
	if shouldSkipByPolicy(memoryDecision("promote", "permanent")) {
		t.Fatal("did not expect permanent promote to skip new memory")
	}
}

func TestShouldRunConflictCheck(t *testing.T) {
	if shouldRunConflictCheck("short", nil) {
		t.Fatal("expected no conflict check when snapshot is empty")
	}
	shortSnapshot := []memorySnapshotEntry{{ID: "a", Layer: "short"}}
	if !shouldRunConflictCheck("short", shortSnapshot) {
		t.Fatal("expected short conflict check with same-layer snapshot")
	}
	longSnapshot := []memorySnapshotEntry{{ID: "b", Layer: "long"}}
	if !shouldRunConflictCheck("short", longSnapshot) {
		t.Fatal("expected short conflict check with longer-lived snapshot")
	}
	instantSnapshot := []memorySnapshotEntry{{ID: "c", Layer: "instant"}}
	if shouldRunConflictCheck("short", instantSnapshot) {
		t.Fatal("did not expect short conflict check against instant-only snapshot")
	}
}

func TestFindMatchingOpenGroupReusesClosedGroupByRefs(t *testing.T) {
	store := memory.NewMemoryStore()
	group := memory.HistoryGroup{
		GroupUUID:    "existing-group",
		Resident:     "onyx",
		CreatedAt:    time.Date(2026, 6, 2, 10, 30, 0, 0, time.UTC),
		ClosedAt:     time.Date(2026, 6, 2, 18, 0, 0, 0, time.UTC),
		LastEventAt:  time.Date(2026, 6, 2, 18, 0, 0, 0, time.UTC),
		SourceKind:   "dialogue_window",
		State:        memory.HistoryGroupClosed,
		CloseReason:  "legacy_closed_group",
		EventCount:   5,
		Tags:         []string{"scenario:baseline", "layer:permanent"},
		RawEventRefs: []string{"baseline:permanent:r3:resource_change", "baseline:permanent:r4:failure", "baseline:permanent:r5:failure", "baseline:permanent:r6:recovery", "baseline:permanent:r7:admin_feedback"},
	}
	if err := store.UpsertHistoryGroup(group); err != nil {
		t.Fatalf("upsert group: %v", err)
	}

	events, err := buildScenario("baseline")
	if err != nil {
		t.Fatalf("build scenario: %v", err)
	}
	window, err := selectWindow(events, "onyx", "permanent")
	if err != nil {
		t.Fatalf("select window: %v", err)
	}

	got := findMatchingOpenGroup(store, "onyx", "baseline", "permanent", window)
	if got.GroupUUID != "existing-group" {
		t.Fatalf("expected existing group, got %q", got.GroupUUID)
	}
}

func TestFindMatchingOpenGroupAppendsCompatibleWindow(t *testing.T) {
	store := memory.NewMemoryStore()
	group := memory.HistoryGroup{
		GroupUUID:    "open-group",
		Resident:     "onyx",
		CreatedAt:    time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC),
		ClosedAt:     time.Date(2026, 6, 2, 10, 30, 0, 0, time.UTC),
		LastEventAt:  time.Date(2026, 6, 2, 10, 30, 0, 0, time.UTC),
		SourceKind:   "dialogue_window",
		State:        memory.HistoryGroupOpen,
		EventCount:   2,
		Tags:         []string{"scenario:baseline", "layer:permanent", "category:resource_change", "high-importance"},
		RawEventRefs: []string{"baseline:permanent:r1:resource_change", "baseline:permanent:r2:failure"},
	}
	if err := store.UpsertHistoryGroup(group); err != nil {
		t.Fatalf("upsert group: %v", err)
	}

	window := []event{
		{Round: 3, Time: time.Date(2026, 6, 2, 11, 0, 0, 0, time.UTC), Category: "resource_change", Importance: 4, Summary: "disk expansion request was approved after evidence was shown"},
		{Round: 4, Time: time.Date(2026, 6, 2, 11, 30, 0, 0, time.UTC), Category: "failure", Importance: 3, Summary: "service bootstrap failed on the first attempt"},
	}

	got := findMatchingOpenGroup(store, "onyx", "baseline", "permanent", window)
	if got.GroupUUID != "open-group" {
		t.Fatalf("expected open group match, got %q", got.GroupUUID)
	}
}

func memoryDecision(action, layer string) memory.Decision {
	return memory.Decision{
		Action:      memory.Action(action),
		TargetLayer: memory.Layer(layer),
	}
}
