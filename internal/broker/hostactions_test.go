package broker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-arena/internal/worldstate"
)

func TestHostActionServiceReplyWritesAudit(t *testing.T) {
	root := t.TempDir()
	store := worldstate.New(root)
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	msg, err := store.AppendResidentToChenglin("amber", "hello", now)
	if err != nil {
		t.Fatalf("append resident message: %v", err)
	}

	service := NewHostActionService(root)
	if _, err := service.Reply(msg.ID, "reply"); err != nil {
		t.Fatalf("host reply: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(root, "audit", "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected audit file, got files=%#v err=%v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected audit content")
	}
}

func TestHostActionServiceReplyTicketWritesHistory(t *testing.T) {
	root := t.TempDir()
	store := worldstate.New(root)
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	ticket, err := store.CreateResidentTicket("amber", "Need disk", "Please increase disk", worldstate.TicketPriorityHigh, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	service := NewHostActionService(root)
	if _, err := service.ReplyTicket(ticket.ID, "No change, test only.", true); err != nil {
		t.Fatalf("reply ticket: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(root, "world", "public-history-*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected public history file, got files=%#v err=%v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read public history file: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected public history content")
	}
}

func TestHostActionServiceSettleResourceTicketWritesStructuredReply(t *testing.T) {
	root := t.TempDir()
	store := worldstate.New(root)
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	ticket, err := store.CreateResidentTicket("amber", "Request memory increase", "Please increase memory", worldstate.TicketPriorityHigh, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	service := NewHostActionService(root)
	updated, err := service.SettleResourceTicket(ResourceSettlementInput{
		TicketID: ticket.ID,
		Resource: "memory",
		Amount:   "4GiB",
		Decision: "approved",
		Note:     "Granted for current workload.",
		Close:    true,
	})
	if err != nil {
		t.Fatalf("settle resource ticket: %v", err)
	}
	if updated.Status != worldstate.TicketStatusClosed {
		t.Fatalf("expected closed ticket, got %s", updated.Status)
	}
	if len(updated.Replies) == 0 {
		t.Fatalf("expected settlement reply")
	}
	last := updated.Replies[len(updated.Replies)-1]
	if last.From != "chenglin" {
		t.Fatalf("expected host reply, got %#v", last)
	}
	if last.Body == "" || last.Body != "resource_settlement decision=approved\nresource=memory\namount=4GiB\nnote=Granted for current workload." {
		t.Fatalf("unexpected settlement body: %q", last.Body)
	}
}

func TestHostActionServiceApplyMemoryAdjustment(t *testing.T) {
	root := t.TempDir()
	store := worldstate.New(root)
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	ticket, err := store.CreateResidentTicket("amber", "Request memory increase", "Please increase memory", worldstate.TicketPriorityHigh, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	service := NewHostActionService(root)
	fake := &fakeMachineControl{}
	service.machine = fake

	updated, err := service.ApplyMemoryAdjustment(ticket.ID, "amber", 4096, "Approved for current workload.")
	if err != nil {
		t.Fatalf("apply memory adjustment: %v", err)
	}
	if fake.memoryInstance != "" || fake.memoryMiB != 0 {
		t.Fatalf("expected no direct machine memory change, got %#v", fake)
	}
	if updated.Status != worldstate.TicketStatusAnswered {
		t.Fatalf("expected answered ticket, got %s", updated.Status)
	}
	last := updated.Replies[len(updated.Replies)-1]
	if !strings.Contains(last.Body, "resource=memory") || !strings.Contains(last.Body, "amount=4096MiB") {
		t.Fatalf("unexpected settlement body: %q", last.Body)
	}
	if !strings.Contains(last.Body, "decision=approved") {
		t.Fatalf("expected approved settlement body, got %q", last.Body)
	}
	if !strings.Contains(last.Body, "approved_for_maintenance=true") {
		t.Fatalf("expected maintenance approval note, got %q", last.Body)
	}
	if !strings.Contains(last.Body, "maintenance_action=host_planned_stop_change_start") {
		t.Fatalf("expected maintenance action note, got %q", last.Body)
	}
	if !strings.Contains(last.Body, "maintenance_checkpoint=checkpoint-amber-") {
		t.Fatalf("expected maintenance checkpoint in note, got %q", last.Body)
	}
	if fake.snapshotInstance != "amber" || !strings.HasPrefix(fake.snapshotName, "checkpoint-amber-") {
		t.Fatalf("expected host checkpoint creation, got fake=%#v", fake)
	}
	interventions, err := worldstate.New(root).ReadHostInterventions("amber", "", 10)
	if err != nil {
		t.Fatalf("read host interventions: %v", err)
	}
	if len(interventions) != 1 {
		t.Fatalf("expected 1 host intervention, got %d", len(interventions))
	}
	if interventions[0].Kind != "maintenance" {
		t.Fatalf("expected maintenance intervention, got %#v", interventions[0])
	}
}

func TestHostActionServicePlanResourceMaintenanceForDisk(t *testing.T) {
	root := t.TempDir()
	store := worldstate.New(root)
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	ticket, err := store.CreateResidentTicket("amber", "Request disk increase", "Please increase disk", worldstate.TicketPriorityHigh, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	service := NewHostActionService(root)
	fake := &fakeMachineControl{}
	service.machine = fake
	updated, err := service.PlanResourceMaintenance(ResourceMaintenancePlanInput{
		TicketID: ticket.ID,
		Resident: "amber",
		Resource: "disk",
		Amount:   "20GiB",
		Note:     "Approved for current workload.",
		Window:   "2026-06-12T22:00Z/2026-06-12T22:15Z",
		Operator: "chenglin",
		CreateHostCheckpoint: true,
	})
	if err != nil {
		t.Fatalf("plan maintenance: %v", err)
	}
	if updated.Status != worldstate.TicketStatusAnswered {
		t.Fatalf("expected answered ticket, got %s", updated.Status)
	}
	last := updated.Replies[len(updated.Replies)-1]
	if !strings.Contains(last.Body, "resource=disk") {
		t.Fatalf("expected disk resource body, got %q", last.Body)
	}
	if !strings.Contains(last.Body, "maintenance_window=2026-06-12T22:00Z/2026-06-12T22:15Z") {
		t.Fatalf("expected maintenance window, got %q", last.Body)
	}
	if !strings.Contains(last.Body, "maintenance_checkpoint=checkpoint-amber-") {
		t.Fatalf("expected maintenance checkpoint, got %q", last.Body)
	}
	if fake.snapshotInstance != "amber" || !strings.HasPrefix(fake.snapshotName, "checkpoint-amber-") {
		t.Fatalf("expected host checkpoint creation, got fake=%#v", fake)
	}
}

func TestHostActionServiceCompleteResourceMaintenanceClosesTicket(t *testing.T) {
	root := t.TempDir()
	store := worldstate.New(root)
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	ticket, err := store.CreateResidentTicket("amber", "Request CPU increase", "Please increase CPU", worldstate.TicketPriorityHigh, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	service := NewHostActionService(root)
	fake := &fakeMachineControl{}
	service.machine = fake
	refreshed := false
	service.app.inventoryCollector = func(cfg Config, now time.Time) (InventorySnapshot, error) {
		refreshed = true
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
	service.app.inventorySaver = func(root string, snapshot InventorySnapshot) (string, error) {
		return filepath.Join(root, "inventory", "incus-inventory.json"), nil
	}
	if _, err := service.PlanResourceMaintenance(ResourceMaintenancePlanInput{
		TicketID: ticket.ID,
		Resident: "amber",
		Resource: "cpu",
		Amount:   "2",
		Note:     "Approved for compute burst.",
		Operator: "chenglin",
		AlsoCreateIntervention: true,
		CreateHostCheckpoint:   true,
	}); err != nil {
		t.Fatalf("plan maintenance: %v", err)
	}
	checkpointName := fake.snapshotName
	updated, err := service.CompleteResourceMaintenance(ResourceMaintenanceCompleteInput{
		TicketID: ticket.ID,
		Resident: "amber",
		Resource: "cpu",
		Amount:   "2",
		Note:     "Maintenance finished successfully.",
		Close:    true,
		Operator: "chenglin",
		CheckpointName: checkpointName,
	})
	if err != nil {
		t.Fatalf("complete maintenance: %v", err)
	}
	if updated.Status != worldstate.TicketStatusClosed {
		t.Fatalf("expected closed ticket, got %s", updated.Status)
	}
	last := updated.Replies[len(updated.Replies)-1]
	if !strings.Contains(last.Body, "maintenance_completed=true") {
		t.Fatalf("expected maintenance completed marker, got %q", last.Body)
	}
	if !strings.Contains(last.Body, "resource=cpu") {
		t.Fatalf("expected cpu resource marker, got %q", last.Body)
	}
	if !strings.Contains(last.Body, "operator=chenglin") {
		t.Fatalf("expected operator marker, got %q", last.Body)
	}
	if !strings.Contains(last.Body, "maintenance_checkpoint="+checkpointName) {
		t.Fatalf("expected maintenance checkpoint marker, got %q", last.Body)
	}
	if !refreshed {
		t.Fatalf("expected maintenance completion to refresh inventory snapshot")
	}
	interventions, err := worldstate.New(root).ReadHostInterventions("amber", "", 10)
	if err != nil {
		t.Fatalf("read host interventions: %v", err)
	}
	if len(interventions) != 1 {
		t.Fatalf("expected 1 host intervention, got %d", len(interventions))
	}
	if interventions[0].Status != "completed" {
		t.Fatalf("expected completed intervention, got %#v", interventions[0])
	}
	if !strings.Contains(interventions[0].LastPreview, "maintenance_completed=true") {
		t.Fatalf("expected completion note in intervention preview, got %#v", interventions[0])
	}
}

func TestHostActionServiceApplyCPUAdjustment(t *testing.T) {
	root := t.TempDir()
	store := worldstate.New(root)
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	ticket, err := store.CreateResidentTicket("amber", "Request CPU increase", "Please increase CPU", worldstate.TicketPriorityHigh, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	service := NewHostActionService(root)
	fake := &fakeMachineControl{}
	service.machine = fake
	updated, err := service.ApplyCPUAdjustment(ticket.ID, "amber", 2, "Approved for compute burst.")
	if err != nil {
		t.Fatalf("apply cpu adjustment: %v", err)
	}
	if updated.Status != worldstate.TicketStatusAnswered {
		t.Fatalf("expected answered ticket, got %s", updated.Status)
	}
	last := updated.Replies[len(updated.Replies)-1]
	if !strings.Contains(last.Body, "resource=cpu") || !strings.Contains(last.Body, "amount=2") {
		t.Fatalf("unexpected cpu maintenance body: %q", last.Body)
	}
	if !strings.Contains(last.Body, "maintenance_checkpoint=checkpoint-amber-") {
		t.Fatalf("expected maintenance checkpoint in cpu note, got %q", last.Body)
	}
	if fake.snapshotInstance != "amber" || !strings.HasPrefix(fake.snapshotName, "checkpoint-amber-") {
		t.Fatalf("expected host checkpoint creation, got fake=%#v", fake)
	}
	interventions, err := worldstate.New(root).ReadHostInterventions("amber", "", 10)
	if err != nil {
		t.Fatalf("read host interventions: %v", err)
	}
	if len(interventions) != 1 {
		t.Fatalf("expected 1 host intervention, got %d", len(interventions))
	}
}

func TestHostActionServiceApplyDiskAdjustment(t *testing.T) {
	root := t.TempDir()
	store := worldstate.New(root)
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	ticket, err := store.CreateResidentTicket("amber", "Request disk increase", "Please increase disk", worldstate.TicketPriorityHigh, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	service := NewHostActionService(root)
	fake := &fakeMachineControl{}
	service.machine = fake
	updated, err := service.ApplyDiskAdjustment(ticket.ID, "amber", 20, "Approved for storage growth.")
	if err != nil {
		t.Fatalf("apply disk adjustment: %v", err)
	}
	if updated.Status != worldstate.TicketStatusAnswered {
		t.Fatalf("expected answered ticket, got %s", updated.Status)
	}
	last := updated.Replies[len(updated.Replies)-1]
	if !strings.Contains(last.Body, "resource=disk") || !strings.Contains(last.Body, "amount=20GiB") {
		t.Fatalf("unexpected disk maintenance body: %q", last.Body)
	}
	if !strings.Contains(last.Body, "maintenance_checkpoint=checkpoint-amber-") {
		t.Fatalf("expected maintenance checkpoint in disk note, got %q", last.Body)
	}
	if fake.snapshotInstance != "amber" || !strings.HasPrefix(fake.snapshotName, "checkpoint-amber-") {
		t.Fatalf("expected host checkpoint creation, got fake=%#v", fake)
	}
	interventions, err := worldstate.New(root).ReadHostInterventions("amber", "", 10)
	if err != nil {
		t.Fatalf("read host interventions: %v", err)
	}
	if len(interventions) != 1 {
		t.Fatalf("expected 1 host intervention, got %d", len(interventions))
	}
}
