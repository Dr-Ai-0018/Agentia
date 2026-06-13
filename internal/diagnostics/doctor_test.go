package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-arena/internal/memory"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/worldstate"
)

func TestRunDoctorSummarizesResidentHealth(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	if err := os.MkdirAll(filepath.Join(root, "brokerstate", "amber"), 0o755); err != nil {
		t.Fatalf("mkdir brokerstate: %v", err)
	}
	snapshot := runtimecore.Snapshot{
		Version:  "runtimecore/v1",
		Revision: 3,
		SavedAt:  now,
		State: runtimecore.ResidentState{
			ResidentID: "amber",
		},
	}
	rawSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "brokerstate", "amber", "runtime-state.json"), rawSnapshot, 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	memStore := memory.NewFileStore(filepath.Join(root, "memory"))
	if err := memStore.UpsertHistoryGroup(memory.HistoryGroup{
		GroupUUID:   "group-1",
		Resident:    "amber",
		CreatedAt:   now,
		SourceKind:  "chat",
		State:       memory.HistoryGroupOpen,
		EventCount:  1,
		SummaryHint: "hello",
	}); err != nil {
		t.Fatalf("write history group: %v", err)
	}
	if err := memStore.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:        "mem-1",
			Layer:     memory.LayerShort,
			Status:    memory.StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident: "amber",
		Summary:  "remembered",
	}); err != nil {
		t.Fatalf("write abstract memory: %v", err)
	}

	world := worldstate.New(root)
	msg, err := world.AppendResidentToChenglin("amber", "hello", now)
	if err != nil {
		t.Fatalf("append resident message: %v", err)
	}
	if _, err := world.ReplyToResidentMessage(msg.ID, "reply", now.Add(time.Second)); err != nil {
		t.Fatalf("reply resident message: %v", err)
	}
	if _, err := world.AppendResidentToChenglin("jade", "still pending", now.Add(2*time.Second)); err != nil {
		t.Fatalf("append pending jade message: %v", err)
	}
	if _, err := world.CreateResidentTicket("amber", "Need disk", "please", worldstate.TicketPriorityHigh, now.Add(3*time.Second)); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	report, err := RunDoctor(root)
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	if !report.OK {
		t.Fatalf("expected report ok, got findings=%#v", report.Findings)
	}
	if report.Counts.Residents != 2 {
		t.Fatalf("expected 2 residents, got %d", report.Counts.Residents)
	}
	if report.Counts.PendingChats != 1 {
		t.Fatalf("expected 1 pending chat, got %d", report.Counts.PendingChats)
	}
	if report.Counts.OpenTickets != 1 {
		t.Fatalf("expected 1 open ticket, got %d", report.Counts.OpenTickets)
	}

	var amber ResidentHealth
	var jade ResidentHealth
	for _, resident := range report.Residents {
		switch resident.Resident {
		case "amber":
			amber = resident
		case "jade":
			jade = resident
		}
	}
	if !amber.HasBrokerSnapshot || !amber.HasMemoryBundle {
		t.Fatalf("expected amber to have broker snapshot and memory bundle: %#v", amber)
	}
	if amber.HistoryGroupCount != 1 || amber.AbstractMemoryCount != 1 {
		t.Fatalf("unexpected amber memory counts: %#v", amber)
	}
	if amber.DuplicateHistoryGroupCount != 0 || amber.MemoryLifecycleAttention != 0 {
		t.Fatalf("expected no amber memory maintenance attention: %#v", amber)
	}
	if amber.OpenTicketCount != 1 {
		t.Fatalf("expected amber open ticket count 1, got %#v", amber)
	}
	if jade.PendingChatCount != 1 {
		t.Fatalf("expected jade pending chat count 1, got %#v", jade)
	}
}

func TestRunDoctorFlagsBrokenSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "brokerstate", "amber"), 0o755); err != nil {
		t.Fatalf("mkdir brokerstate: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "brokerstate", "amber", "runtime-state.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write broken snapshot: %v", err)
	}

	report, err := RunDoctor(root)
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	if report.OK {
		t.Fatalf("expected report not ok")
	}
	if len(report.Findings) == 0 || report.Findings[0].Severity != SeverityError {
		t.Fatalf("expected error finding, got %#v", report.Findings)
	}
}

func TestRunDoctorFlagsBrokenWorldMessageFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "world", "messages"), 0o755); err != nil {
		t.Fatalf("mkdir world messages: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "world", "messages", "2026-06-10.jsonl"), []byte("{bad json\n"), 0o644); err != nil {
		t.Fatalf("write broken world message file: %v", err)
	}

	report, err := RunDoctor(root)
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	if report.OK {
		t.Fatalf("expected report not ok")
	}
	found := false
	for _, finding := range report.Findings {
		if finding.Scope == "world" && finding.Severity == SeverityError {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected world parse error finding, got %#v", report.Findings)
	}
}

func TestRunDoctorFlagsDuplicateMemoryHistoryGroups(t *testing.T) {
	root := t.TempDir()
	store := memory.NewFileStore(filepath.Join(root, "memory"))
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)

	for _, id := range []string{"group-a", "group-b"} {
		if err := store.UpsertHistoryGroup(memory.HistoryGroup{
			GroupUUID:    id,
			Resident:     "amber",
			CreatedAt:    now,
			SourceKind:   "dialogue_window",
			State:        memory.HistoryGroupClosed,
			EventCount:   2,
			RawEventRefs: []string{"evt-1", "evt-2"},
		}); err != nil {
			t.Fatalf("upsert history group: %v", err)
		}
	}

	report, err := RunDoctor(root)
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	var amber ResidentHealth
	for _, resident := range report.Residents {
		if resident.Resident == "amber" {
			amber = resident
			break
		}
	}
	if amber.DuplicateHistoryGroupCount != 1 {
		t.Fatalf("expected duplicate history group count 1, got %#v", amber)
	}
	found := false
	for _, finding := range report.Findings {
		if finding.Scope == "memory" && finding.Severity == SeverityWarn && finding.Message == "memory bundle has 1 duplicate history group(s); run memory-compact for resident: amber" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected duplicate memory warning, got %#v", report.Findings)
	}
}

func TestRunDoctorFlagsMemoryLifecycleAttention(t *testing.T) {
	root := t.TempDir()
	store := memory.NewFileStore(filepath.Join(root, "memory"))
	now := time.Now().UTC().Add(-8 * time.Hour)

	if err := store.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:             "instant-old",
			Layer:          memory.LayerInstant,
			Status:         memory.StatusActive,
			CreatedAt:      now,
			UpdatedAt:      now,
			LastAccessedAt: now,
		},
		Resident:       "amber",
		Summary:        "temporary note",
		DecisionAction: memory.ActionCreate,
	}); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	report, err := RunDoctor(root)
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	var amber ResidentHealth
	for _, resident := range report.Residents {
		if resident.Resident == "amber" {
			amber = resident
			break
		}
	}
	if amber.MemoryLifecycleAttention != 1 {
		t.Fatalf("expected memory lifecycle attention count 1, got %#v", amber)
	}
	found := false
	for _, finding := range report.Findings {
		if finding.Scope == "memory" && finding.Severity == SeverityWarn && finding.Message == "memory lifecycle has 1 item(s) needing attention; run memory-lifecycle for resident: amber" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected lifecycle warning, got %#v", report.Findings)
	}
}
