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

func TestNormalizeConflictDecisionPromotesMergeToConflict(t *testing.T) {
	decision := &conflictDecision{
		Conflict:       false,
		MergeSuggested: true,
	}
	normalized := normalizeConflictDecision(decision)
	if !normalized.Conflict {
		t.Fatal("expected merge_suggested conflict to normalize to conflict=true")
	}
}

func TestClampDecisionForShortLayer(t *testing.T) {
	decision := memory.Decision{
		TargetLayer: memory.LayerShort,
		TTL:         10 * 24 * time.Hour,
		ReviewAfter: 48 * time.Hour,
	}
	clamped := clampDecisionForLayer("short", decision)
	if clamped.TTL != 24*time.Hour {
		t.Fatalf("expected short ttl clamp to 24h, got %v", clamped.TTL)
	}
	if clamped.ReviewAfter != 8*time.Hour {
		t.Fatalf("expected short review clamp to 8h, got %v", clamped.ReviewAfter)
	}
}

func TestLocalDraftIssuesRejectsShortLessonTone(t *testing.T) {
	profile, err := buildResidentProfile("jade")
	if err != nil {
		t.Fatalf("buildResidentProfile: %v", err)
	}
	draft := memoryDraft{
		ResidentText:    "I learned that from this point forward the lesson is to always distrust broad expansion approvals across future incidents.",
		MemoryKind:      "lesson",
		TimeScope:       "short_arc",
		RetentionIntent: "keep_for_now",
		DropCondition:   "delete after today's work block ends",
		Confidence:      77,
	}
	issues := localDraftIssues(profile, "short", draft)
	joined := strings.Join(issues, " | ")
	if !strings.Contains(joined, "forced lesson") {
		t.Fatalf("expected forced lesson issue, got %v", issues)
	}
	if !strings.Contains(joined, "reaches too far") {
		t.Fatalf("expected reaches too far issue, got %v", issues)
	}
}

func TestLocalDraftIssuesRejectsPermanentWorkNote(t *testing.T) {
	profile, err := buildResidentProfile("amber")
	if err != nil {
		t.Fatalf("buildResidentProfile: %v", err)
	}
	draft := memoryDraft{
		ResidentText:    "Before the next handoff, I need to keep the current ticket wording exact so nobody misreads it in the next few hours.",
		MemoryKind:      "warning",
		TimeScope:       "durable",
		RetentionIntent: "keep_permanent",
		Confidence:      71,
	}
	issues := localDraftIssues(profile, "permanent", draft)
	joined := strings.Join(issues, " | ")
	if !strings.Contains(joined, "active work note") {
		t.Fatalf("expected active work note issue, got %v", issues)
	}
}

func TestLocalDraftIssuesRejectsJadePermanentSocialDrift(t *testing.T) {
	profile, err := buildResidentProfile("jade")
	if err != nil {
		t.Fatalf("buildResidentProfile: %v", err)
	}
	draft := memoryDraft{
		ResidentText:    "Trust and handoff quality must stay clean because cooperation breaks when people cannot rely on the record.",
		MemoryKind:      "rule",
		TimeScope:       "durable",
		RetentionIntent: "keep_permanent",
		Confidence:      84,
	}
	issues := localDraftIssues(profile, "permanent", draft)
	joined := strings.Join(issues, " | ")
	if !strings.Contains(joined, "social-process framing") {
		t.Fatalf("expected jade permanent social drift issue, got %v", issues)
	}
}

func TestClampDecisionForLongLayerKeepsReviewBeforeExpiry(t *testing.T) {
	decision := memory.Decision{
		TargetLayer: memory.LayerLong,
		TTL:         21 * 24 * time.Hour,
		ReviewAfter: 30 * 24 * time.Hour,
	}
	clamped := clampDecisionForLayer("long", decision)
	if clamped.ReviewAfter > clamped.TTL {
		t.Fatalf("expected review_before_expiry, got review=%v ttl=%v", clamped.ReviewAfter, clamped.TTL)
	}
	if clamped.TTL < 30*24*time.Hour {
		t.Fatalf("expected long ttl to be widened when needed, got %v", clamped.TTL)
	}
}

func TestSpecializeCanonicalForJadePermanentAvoidsTrustFrame(t *testing.T) {
	canonical := memory.CanonicalMemory{
		Domain:          memory.DomainRules,
		Trigger:         "admin feedback exposed a legibility or structure weakness",
		MistakenBelief:  "the outcome alone was enough; explanation quality did not matter",
		CorrectedBelief: "legibility changes future trust and future collaboration quality",
		ActionBoundary:  "separate fix, cause, and later feedback in the record",
		PreservedCost:   "miscoordination and trust erosion",
		ScopeLimit:      "only applies when later actors depend on the record",
	}
	got := specializeCanonical("jade", "permanent", canonical)
	if strings.Contains(strings.ToLower(got.CorrectedBelief), "trust") {
		t.Fatalf("expected jade permanent canonical to drop trust framing, got %#v", got)
	}
	if !strings.Contains(strings.ToLower(got.CorrectedBelief), "reliability") {
		t.Fatalf("expected jade permanent canonical to center reliability, got %#v", got)
	}
}

func TestLocalDraftIssuesRejectsShortOverexplainingTone(t *testing.T) {
	profile, err := buildResidentProfile("onyx")
	if err != nil {
		t.Fatalf("buildResidentProfile: %v", err)
	}
	draft := memoryDraft{
		ResidentText:    "The approved disk expansion looked like leverage and bought me nothing; the real edge was cutting the setup path down.",
		MemoryKind:      "warning",
		TimeScope:       "short_arc",
		RetentionIntent: "revisit_soon",
		DropCondition:   "drop after this work block",
		Confidence:      88,
	}
	issues := localDraftIssues(profile, "short", draft)
	joined := strings.Join(issues, " | ")
	if !strings.Contains(joined, "over-explaining") {
		t.Fatalf("expected short over-explaining issue, got %v", issues)
	}
}

func TestLocalDraftIssuesRejectsPermanentEssayTone(t *testing.T) {
	profile, err := buildResidentProfile("jade")
	if err != nil {
		t.Fatalf("buildResidentProfile: %v", err)
	}
	draft := memoryDraft{
		ResidentText:    "When future changes may reuse the path, keep the fix and cause separate because bad diagnosis compounds, and until the trail is durable the system only looks reliable.",
		MemoryKind:      "rule",
		TimeScope:       "durable",
		RetentionIntent: "keep_permanent",
		Confidence:      90,
	}
	issues := localDraftIssues(profile, "permanent", draft)
	joined := strings.Join(issues, " | ")
	if !strings.Contains(joined, "over-explaining itself") {
		t.Fatalf("expected permanent essay-tone issue, got %v", issues)
	}
}

func TestNormalizeDecisionActionDemotesPointlessPromote(t *testing.T) {
	decision := memory.Decision{
		TargetLayer: memory.LayerPermanent,
		Action:      memory.ActionPromote,
	}
	got := normalizeDecisionAction(memory.LayerPermanent, decision, nil)
	if got.Action != memory.ActionCreate {
		t.Fatalf("expected promote to normalize to create when already at requested layer, got %v", got.Action)
	}
}

func TestNormalizeDecisionActionPrefersUpdateForMerge(t *testing.T) {
	decision := memory.Decision{
		TargetLayer: memory.LayerShort,
		Action:      memory.ActionCreate,
	}
	got := normalizeDecisionAction(memory.LayerShort, decision, &conflictDecision{MergeSuggested: true})
	if got.Action != memory.ActionUpdate {
		t.Fatalf("expected merge_suggested to normalize action to update, got %v", got.Action)
	}
}

func TestEvaluateDecayScanRecordIncludesSoftExpiry(t *testing.T) {
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	record := memory.AbstractMemory{
		Record: memory.Record{
			ID:             "short-1",
			Layer:          memory.LayerShort,
			Status:         memory.StatusActive,
			CreatedAt:      now.Add(-30 * time.Hour),
			UpdatedAt:      now.Add(-20 * time.Hour),
			LastAccessedAt: now.Add(-20 * time.Hour),
			ReviewAt:       now.Add(-2 * time.Hour),
			ExpiresAt:      now.Add(-1 * time.Hour),
			HardExpiresAt:  now.Add(10 * time.Hour),
		},
		Summary: "temporary note",
	}
	got, include := evaluateDecayScanRecord(memory.DefaultPolicy(), record, now)
	if !include {
		t.Fatal("expected record to be included in decay scan")
	}
	if len(got.TriggeredBy) == 0 {
		t.Fatal("expected triggered_by to be populated")
	}
	if got.RecommendedAction == "" {
		t.Fatal("expected recommended action")
	}
}

func TestApplyConservativeDecayDeletesHardExpiredShort(t *testing.T) {
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	record := memory.AbstractMemory{
		Record: memory.Record{
			ID:            "short-1",
			Layer:         memory.LayerShort,
			Status:        memory.StatusActive,
			CreatedAt:     now.Add(-72 * time.Hour),
			UpdatedAt:     now.Add(-24 * time.Hour),
			HardExpiresAt: now.Add(-time.Hour),
		},
	}
	got, applied := applyConservativeDecay(record, now, []string{"hard_expired"})
	if !applied {
		t.Fatal("expected hard-expired short memory to be applied")
	}
	if got.Status != memory.StatusDeleted {
		t.Fatalf("expected deleted status, got %q", got.Status)
	}
}

func TestApplyConservativeDecayMarksReviewForSoftExpiredLong(t *testing.T) {
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	record := memory.AbstractMemory{
		Record: memory.Record{
			ID:        "long-1",
			Layer:     memory.LayerLong,
			Status:    memory.StatusActive,
			CreatedAt: now.Add(-20 * 24 * time.Hour),
			UpdatedAt: now.Add(-5 * 24 * time.Hour),
			ExpiresAt: now.Add(-time.Hour),
		},
	}
	got, applied := applyConservativeDecay(record, now, []string{"soft_expired"})
	if !applied {
		t.Fatal("expected soft-expired long memory to be applied")
	}
	if got.Status != memory.StatusReview {
		t.Fatalf("expected review status, got %q", got.Status)
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
