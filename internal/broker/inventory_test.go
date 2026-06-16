package broker

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"ai-arena/internal/worldstate"
)

func TestSaveInventorySnapshot(t *testing.T) {
	root := t.TempDir()
	snapshot := InventorySnapshot{
		CollectedAt: time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Residents: []ResidentInventoryFact{
			{
				ResidentID:     "amber",
				InstanceName:   "amber",
				Status:         "Running",
				Type:           "virtual-machine",
				VCPU:           1,
				MemoryLimitMiB: 2048,
				DiskGiB:        12,
				IPv4:           "10.0.0.2",
				UpdatedAt:      time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC).Format(time.RFC3339),
			},
		},
	}

	path, err := SaveInventorySnapshot(root, snapshot)
	if err != nil {
		t.Fatalf("save inventory snapshot: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat inventory snapshot: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read inventory snapshot: %v", err)
	}
	if !strings.Contains(string(raw), `"resident_id": "amber"`) {
		t.Fatalf("expected amber resident in snapshot, got %s", string(raw))
	}
	if filepath.Base(path) != "incus-inventory.json" {
		t.Fatalf("unexpected snapshot filename: %s", path)
	}
}

func TestParseMemoryLimitMiB(t *testing.T) {
	if got := parseMemoryLimitMiB("2GiB", 0); got != 2048 {
		t.Fatalf("expected 2048 MiB, got %d", got)
	}
	if got := parseMemoryLimitMiB("2304MiB", 0); got != 2304 {
		t.Fatalf("expected 2304 MiB, got %d", got)
	}
	if got := parseMemoryLimitMiB("bad", 123); got != 123 {
		t.Fatalf("expected fallback 123, got %d", got)
	}
}

func TestExtractIPv4SkipsLoopback(t *testing.T) {
	item := struct {
		Name           string            `json:"name"`
		Status         string            `json:"status"`
		Type           string            `json:"type"`
		Config         map[string]string `json:"config"`
		ExpandedConfig map[string]string `json:"expanded_config"`
		State          struct {
			Network map[string]struct {
				Addresses []struct {
					Family  string `json:"family"`
					Address string `json:"address"`
				} `json:"addresses"`
			} `json:"network"`
		} `json:"state"`
	}{}
	item.State.Network = map[string]struct {
		Addresses []struct {
			Family  string `json:"family"`
			Address string `json:"address"`
		} `json:"addresses"`
	}{
		"lo": {
			Addresses: []struct {
				Family  string `json:"family"`
				Address string `json:"address"`
			}{
				{Family: "inet", Address: "127.0.0.1"},
			},
		},
		"eth0": {
			Addresses: []struct {
				Family  string `json:"family"`
				Address string `json:"address"`
			}{
				{Family: "inet", Address: "10.244.206.102"},
			},
		},
	}
	if got := extractIPv4(item); got != "10.244.206.102" {
		t.Fatalf("expected non-loopback ipv4, got %q", got)
	}
}

func TestRunHostInspectAggregatesWorldState(t *testing.T) {
	root := t.TempDir()
	app := New(root)
	world := worldstate.New(root)
	now := time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC)

	if _, err := world.AppendResidentToChenglin("amber", "hello", now); err != nil {
		t.Fatalf("append resident message: %v", err)
	}
	if _, err := world.CreateResidentTicket("amber", "Need help", "Please inspect this state", worldstate.TicketPriorityMedium, now); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := SaveInventorySnapshot(root, InventorySnapshot{
		CollectedAt: now.Format(time.RFC3339),
		Residents: []ResidentInventoryFact{
			{
				ResidentID:     "amber",
				InstanceName:   "amber",
				Status:         "Running",
				Type:           "virtual-machine",
				VCPU:           1,
				MemoryLimitMiB: 2048,
				DiskGiB:        12,
				IPv4:           "10.0.0.2",
				UpdatedAt:      now.Format(time.RFC3339),
			},
		},
	}); err != nil {
		t.Fatalf("save inventory snapshot: %v", err)
	}
	writeTestOrchestratorInspection(t, root, `{
		"run_id": "orchestrator-20260616T082449.311075354Z",
		"mode": "parallel",
		"residents_finished": 3,
		"residents_errored": 0,
		"useful_runs": 3,
		"budget_blocked_runs": 1,
		"residents": [
			{"resident":"amber","status":"ok","rounds":2,"stopped_reason":"broker_preflight_denied: effective_window_exhausted","budget_blocked":true}
		]
	}`)

	out, err := app.RunHostInspectFromSnapshot(10)
	if err != nil {
		t.Fatalf("run host inspect: %v", err)
	}
	if out.Path == "" {
		t.Fatalf("expected inventory snapshot path")
	}
	if len(out.Inbox.PendingChatMessages) != 1 {
		t.Fatalf("expected 1 pending chat, got %d", len(out.Inbox.PendingChatMessages))
	}
	if len(out.Inbox.OpenTickets) != 1 {
		t.Fatalf("expected 1 open ticket, got %d", len(out.Inbox.OpenTickets))
	}
	if len(out.ResidentFacts) != 3 {
		t.Fatalf("expected runtime facts for default residents, got %d", len(out.ResidentFacts))
	}
	if out.LatestOrchestrator == nil || out.LatestOrchestrator.BudgetBlockedRuns != 1 {
		t.Fatalf("expected latest orchestrator report, got %#v", out.LatestOrchestrator)
	}
	if len(out.RecentOrchestrators) != 1 {
		t.Fatalf("expected recent orchestrator report, got %#v", out.RecentOrchestrators)
	}
	var amber ResidentRuntimeFact
	for _, item := range out.ResidentFacts {
		if item.ResidentID == "amber" {
			amber = item
			break
		}
	}
	if amber.ResidentID == "" {
		t.Fatalf("expected amber runtime fact")
	}
	if amber.ObservedMemoryMiB != 2048 || amber.ConfiguredMemoryMiB != 2048 {
		t.Fatalf("unexpected amber memory fact: %#v", amber)
	}
}

func TestRunHostInspectSummaryFromSnapshot(t *testing.T) {
	root := t.TempDir()
	app := New(root)
	world := worldstate.New(root)
	now := time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC)

	if _, err := world.AppendResidentToChenglin("amber", "hello", now); err != nil {
		t.Fatalf("append resident message: %v", err)
	}
	if _, err := world.CreateResidentTicket("amber", "Need help", "Please inspect this state", worldstate.TicketPriorityMedium, now); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := SaveInventorySnapshot(root, InventorySnapshot{
		CollectedAt: now.Format(time.RFC3339),
		Residents: []ResidentInventoryFact{
			{
				ResidentID:     "amber",
				InstanceName:   "amber",
				Status:         "Running",
				Type:           "virtual-machine",
				VCPU:           2,
				MemoryLimitMiB: 2304,
				DiskGiB:        12,
				IPv4:           "10.0.0.2",
				UpdatedAt:      now.Format(time.RFC3339),
			},
		},
	}); err != nil {
		t.Fatalf("save inventory snapshot: %v", err)
	}
	writeTestOrchestratorInspection(t, root, `{
		"run_id": "orchestrator-20260616T082449.311075354Z",
		"residents_errored": 1,
		"budget_blocked_runs": 1,
		"residents": [
			{"resident":"amber","status":"error","error":"runner failed","budget_blocked":true}
		]
	}`)

	out, err := app.RunHostInspectSummaryFromSnapshot(10)
	if err != nil {
		t.Fatalf("run host inspect summary: %v", err)
	}
	if out.ResidentCount != 3 {
		t.Fatalf("expected summary for default residents, got %#v", out)
	}
	if out.OpenTicketCount != 1 || out.PendingChatResidents != 1 {
		t.Fatalf("unexpected followup counts: %#v", out)
	}
	if out.ResidentsWithDrift == 0 {
		t.Fatalf("expected drift to be detected from snapshot: %#v", out)
	}
	if out.LatestOrchestrator == nil || !out.LatestRunNeedsAttention {
		t.Fatalf("expected latest orchestrator attention in summary: %#v", out)
	}
	if len(out.RecentOrchestrators) != 1 || out.RecentRunsNeedingAttention != 1 {
		t.Fatalf("expected recent orchestrator attention in summary: %#v", out)
	}
}

func writeTestOrchestratorInspection(t *testing.T, root, report string) {
	t.Helper()
	dir := filepath.Join(root, "orchestrator-runs", "orchestrator-20260616T082449.311075354Z")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir orchestrator report dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "inspection-report.json"), []byte(report), 0o644); err != nil {
		t.Fatalf("write orchestrator report: %v", err)
	}
}

func TestBuildResidentRuntimeFactsMarksDrift(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	now := time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC)
	facts := BuildResidentRuntimeFacts(cfg, InventorySnapshot{
		CollectedAt: now.Format(time.RFC3339),
		Residents: []ResidentInventoryFact{
			{
				ResidentID:     "amber",
				InstanceName:   "amber",
				Status:         "Running",
				Type:           "virtual-machine",
				VCPU:           2,
				MemoryLimitMiB: 4096,
				DiskGiB:        20,
				IPv4:           "10.0.0.2",
				UpdatedAt:      now.Format(time.RFC3339),
			},
		},
	})
	var amber ResidentRuntimeFact
	for _, item := range facts {
		if item.ResidentID == "amber" {
			amber = item
			break
		}
	}
	if amber.ResidentID == "" {
		t.Fatalf("expected amber runtime fact")
	}
	if !slices.Equal(amber.DriftFields, []string{"vcpu", "memory", "disk"}) {
		t.Fatalf("expected full drift markers, got %#v", amber.DriftFields)
	}
}

func TestRefreshInventorySnapshotUsesInjectedCollectorAndSaver(t *testing.T) {
	root := t.TempDir()
	app := New(root)
	now := time.Date(2026, 6, 12, 6, 0, 0, 0, time.UTC)
	calledCollector := false
	calledSaver := false
	app.inventoryCollector = func(cfg Config, got time.Time) (InventorySnapshot, error) {
		calledCollector = true
		if !got.Equal(now) {
			t.Fatalf("unexpected collector time: %s", got)
		}
		return InventorySnapshot{
			CollectedAt: now.Format(time.RFC3339),
			Residents: []ResidentInventoryFact{{
				ResidentID:     "amber",
				InstanceName:   "amber",
				Status:         "Running",
				Type:           "virtual-machine",
				VCPU:           1,
				MemoryLimitMiB: 2048,
				DiskGiB:        12,
				UpdatedAt:      now.Format(time.RFC3339),
			}},
		}, nil
	}
	app.inventorySaver = func(gotRoot string, snapshot InventorySnapshot) (string, error) {
		calledSaver = true
		if gotRoot != root {
			t.Fatalf("unexpected saver root: %s", gotRoot)
		}
		if snapshot.CollectedAt != now.Format(time.RFC3339) {
			t.Fatalf("unexpected snapshot passed to saver: %#v", snapshot)
		}
		return filepath.Join(root, "inventory", "incus-inventory.json"), nil
	}

	snapshot, path, err := app.RefreshInventorySnapshot(now)
	if err != nil {
		t.Fatalf("refresh inventory snapshot: %v", err)
	}
	if !calledCollector || !calledSaver {
		t.Fatalf("expected collector and saver to be called")
	}
	if path == "" || snapshot.CollectedAt != now.Format(time.RFC3339) {
		t.Fatalf("unexpected refresh result: path=%q snapshot=%#v", path, snapshot)
	}
}
