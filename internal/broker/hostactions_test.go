package broker

import (
	"encoding/json"
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

func TestHostActionServiceResolveHostIntervention(t *testing.T) {
	root := t.TempDir()
	service := NewHostActionService(root)
	created, err := service.CreateHostIntervention(HostInterventionInput{
		Resident: "amber",
		Kind:     "notice",
		Title:    "Host intervention smoke test",
		Body:     "This is a smoke test.",
		Operator: "chenglin",
	})
	if err != nil {
		t.Fatalf("create host intervention: %v", err)
	}

	resolved, err := service.ResolveHostIntervention(HostInterventionResolveInput{
		InterventionID: created.ID,
		Body:           "Smoke test closed.",
		Operator:       "chenglin",
	})
	if err != nil {
		t.Fatalf("resolve host intervention: %v", err)
	}
	if resolved.Status != "completed" || resolved.Body != "Smoke test closed." {
		t.Fatalf("unexpected resolved intervention: %#v", resolved)
	}
	followups, err := worldstate.New(root).ReadHostFollowups(10)
	if err != nil {
		t.Fatalf("read host followups: %v", err)
	}
	if len(followups) != 0 {
		t.Fatalf("expected resolved intervention to leave followups, got %#v", followups)
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
		TicketID:               ticket.ID,
		Resident:               "amber",
		Resource:               "disk",
		Amount:                 "20GiB",
		Note:                   "Approved for current workload.",
		Window:                 "2026-06-12T22:00Z/2026-06-12T22:15Z",
		Operator:               "chenglin",
		AlsoCreateIntervention: true,
		CreateHostCheckpoint:   true,
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
	interventions, err := worldstate.New(root).ReadHostInterventions("amber", "", 10)
	if err != nil {
		t.Fatalf("read host interventions: %v", err)
	}
	if len(interventions) != 1 {
		t.Fatalf("expected 1 host intervention, got %d", len(interventions))
	}
	if interventions[0].Status != "planned" || interventions[0].Kind != "maintenance" {
		t.Fatalf("expected planned maintenance intervention, got %#v", interventions[0])
	}
}

func TestHostActionServiceHostOnlyMaintenanceLifecycle(t *testing.T) {
	root := t.TempDir()
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

	planned, err := service.PlanHostResourceMaintenance(HostResourceMaintenanceInput{
		Resident:             "amber",
		Resource:             "cpu",
		Amount:               "2",
		Note:                 "Host-initiated maintenance for approved capacity normalization.",
		Window:               "2026-06-19T02:00Z/2026-06-19T02:15Z",
		Operator:             "chenglin",
		CreateHostCheckpoint: true,
	})
	if err != nil {
		t.Fatalf("plan host maintenance: %v", err)
	}
	if planned.Intervention.ID == "" || planned.Intervention.Status != "planned" {
		t.Fatalf("expected planned host intervention, got %#v", planned)
	}
	if !strings.Contains(planned.Intervention.Body, "approved_for_maintenance=true") ||
		!strings.Contains(planned.Intervention.Body, "resource=cpu") ||
		!strings.Contains(planned.Intervention.Body, "amount=2") {
		t.Fatalf("unexpected host maintenance body: %q", planned.Intervention.Body)
	}
	if fake.snapshotInstance != "amber" || !strings.HasPrefix(fake.snapshotName, "checkpoint-amber-") {
		t.Fatalf("expected host checkpoint creation, got fake=%#v", fake)
	}

	started, err := service.StartHostResourceMaintenance(HostResourceMaintenanceInput{
		InterventionID: planned.Intervention.ID,
		Resident:       "amber",
		Resource:       "cpu",
		Amount:         "2",
		Note:           "Maintenance window has started.",
		Operator:       "chenglin",
		CheckpointName: planned.CheckpointName,
	})
	if err != nil {
		t.Fatalf("start host maintenance: %v", err)
	}
	if started.Intervention.Status != "in_progress" || !strings.Contains(started.Intervention.Body, "maintenance_started=true") {
		t.Fatalf("expected in-progress intervention, got %#v", started.Intervention)
	}

	completed, err := service.CompleteHostResourceMaintenance(HostResourceMaintenanceInput{
		InterventionID: planned.Intervention.ID,
		Resident:       "amber",
		Resource:       "cpu",
		Amount:         "2",
		Note:           "Maintenance finished successfully.",
		Operator:       "chenglin",
		CheckpointName: planned.CheckpointName,
	})
	if err != nil {
		t.Fatalf("complete host maintenance: %v", err)
	}
	if completed.Intervention.Status != "completed" || !completed.InventoryRefreshed || !refreshed {
		t.Fatalf("expected completed intervention and inventory refresh, got %#v refreshed=%v", completed, refreshed)
	}
	if !strings.Contains(completed.Intervention.Body, "maintenance_completed=true") {
		t.Fatalf("expected completion marker, got %q", completed.Intervention.Body)
	}
	if !strings.Contains(completed.Intervention.Body, "maintenance_result=host_stop_change_start_finished") ||
		!strings.Contains(completed.Intervention.Body, "approved resource change has been applied") {
		t.Fatalf("expected real maintenance completion wording, got %q", completed.Intervention.Body)
	}
	records := readMaintenanceRunRecords(t, root)
	if len(records) != 3 {
		t.Fatalf("expected planned, in_progress, completed records, got %#v", records)
	}
	if records[0].TicketID != "" || records[0].InterventionID != planned.Intervention.ID {
		t.Fatalf("expected host-only maintenance record without ticket id, got %#v", records[0])
	}
	if records[0].State != "planned" || records[1].State != "in_progress" || records[2].State != "completed" {
		t.Fatalf("unexpected maintenance record states: %#v", records)
	}
	if !records[2].InventoryRefreshed {
		t.Fatalf("expected completion record inventory refresh: %#v", records[2])
	}
}

func TestHostActionServiceHostOnlyNoopMaintenanceCompletionWording(t *testing.T) {
	root := t.TempDir()
	service := NewHostActionService(root)
	service.app.inventoryCollector = func(cfg Config, now time.Time) (InventorySnapshot, error) {
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

	planned, err := service.PlanHostResourceMaintenance(HostResourceMaintenanceInput{
		Resident: "amber",
		Resource: "memory",
		Amount:   "smoke-noop",
		Note:     "Host-only lifecycle smoke. No VM resource change will be performed.",
		Window:   "safe smoke window",
		Operator: "codex",
	})
	if err != nil {
		t.Fatalf("plan host maintenance: %v", err)
	}
	completed, err := service.CompleteHostResourceMaintenance(HostResourceMaintenanceInput{
		InterventionID: planned.Intervention.ID,
		Resident:       "amber",
		Resource:       "memory",
		Amount:         "smoke-noop",
		Note:           "Smoke validation complete with no resource configuration change.",
		Operator:       "codex",
	})
	if err != nil {
		t.Fatalf("complete noop host maintenance: %v", err)
	}
	if !strings.Contains(completed.Intervention.Body, "maintenance_result=host_noop_validation_finished") {
		t.Fatalf("expected noop maintenance completion result, got %q", completed.Intervention.Body)
	}
	if strings.Contains(completed.Intervention.Body, "approved resource change has been applied") {
		t.Fatalf("noop completion must not claim a resource change was applied: %q", completed.Intervention.Body)
	}
	if !strings.Contains(completed.Intervention.Body, "no VM resource configuration was changed") {
		t.Fatalf("expected explicit no-resource-change resident expectation, got %q", completed.Intervention.Body)
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
		TicketID:               ticket.ID,
		Resident:               "amber",
		Resource:               "cpu",
		Amount:                 "2",
		Note:                   "Approved for compute burst.",
		Operator:               "chenglin",
		AlsoCreateIntervention: true,
		CreateHostCheckpoint:   true,
	}); err != nil {
		t.Fatalf("plan maintenance: %v", err)
	}
	checkpointName := fake.snapshotName
	updated, err := service.CompleteResourceMaintenance(ResourceMaintenanceCompleteInput{
		TicketID:       ticket.ID,
		Resident:       "amber",
		Resource:       "cpu",
		Amount:         "2",
		Note:           "Maintenance finished successfully.",
		Close:          true,
		Operator:       "chenglin",
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
	records := readMaintenanceRunRecords(t, root)
	if len(records) != 2 {
		t.Fatalf("expected planned and completed maintenance records, got %#v", records)
	}
	if records[0].State != "planned" || records[1].State != "completed" {
		t.Fatalf("unexpected maintenance record states: %#v", records)
	}
	if records[1].CheckpointName != checkpointName || !records[1].InventoryRefreshed {
		t.Fatalf("expected completion record checkpoint and inventory refresh: %#v", records[1])
	}
}

func TestHostActionServiceStartResourceMaintenanceMarksInterventionInProgress(t *testing.T) {
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
	if _, err := service.PlanResourceMaintenance(ResourceMaintenancePlanInput{
		TicketID:               ticket.ID,
		Resident:               "amber",
		Resource:               "cpu",
		Amount:                 "2",
		Note:                   "Approved for compute burst.",
		Operator:               "chenglin",
		AlsoCreateIntervention: true,
		CreateHostCheckpoint:   true,
	}); err != nil {
		t.Fatalf("plan maintenance: %v", err)
	}
	checkpointName := fake.snapshotName

	updated, err := service.StartResourceMaintenance(ResourceMaintenanceStartInput{
		TicketID:       ticket.ID,
		Resident:       "amber",
		Resource:       "cpu",
		Amount:         "2",
		Note:           "Maintenance window has started.",
		Operator:       "chenglin",
		CheckpointName: checkpointName,
	})
	if err != nil {
		t.Fatalf("start maintenance: %v", err)
	}
	last := updated.Replies[len(updated.Replies)-1]
	if !strings.Contains(last.Body, "maintenance_started=true") {
		t.Fatalf("expected maintenance started marker, got %q", last.Body)
	}
	if !strings.Contains(last.Body, "maintenance_state=in_progress") {
		t.Fatalf("expected maintenance in-progress marker, got %q", last.Body)
	}
	interventions, err := worldstate.New(root).ReadHostInterventions("amber", "", 10)
	if err != nil {
		t.Fatalf("read host interventions: %v", err)
	}
	if len(interventions) != 1 {
		t.Fatalf("expected 1 host intervention, got %d", len(interventions))
	}
	if interventions[0].Status != "in_progress" {
		t.Fatalf("expected in_progress intervention, got %#v", interventions[0])
	}
}

func TestHostActionServiceFailResourceMaintenanceMarksInterventionFailed(t *testing.T) {
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
	if _, err := service.PlanResourceMaintenance(ResourceMaintenancePlanInput{
		TicketID:               ticket.ID,
		Resident:               "amber",
		Resource:               "cpu",
		Amount:                 "2",
		Note:                   "Approved for compute burst.",
		Operator:               "chenglin",
		AlsoCreateIntervention: true,
		CreateHostCheckpoint:   true,
	}); err != nil {
		t.Fatalf("plan maintenance: %v", err)
	}
	checkpointName := fake.snapshotName

	updated, err := service.FailResourceMaintenance(ResourceMaintenanceFailedInput{
		TicketID:       ticket.ID,
		Resident:       "amber",
		Resource:       "cpu",
		Amount:         "2",
		Note:           "Maintenance could not complete because the instance failed to restart.",
		Operator:       "chenglin",
		CheckpointName: checkpointName,
	})
	if err != nil {
		t.Fatalf("fail maintenance: %v", err)
	}
	last := updated.Replies[len(updated.Replies)-1]
	if !strings.Contains(last.Body, "maintenance_failed=true") {
		t.Fatalf("expected maintenance failed marker, got %q", last.Body)
	}
	if !strings.Contains(last.Body, "maintenance_state=failed") {
		t.Fatalf("expected maintenance failed state, got %q", last.Body)
	}
	interventions, err := worldstate.New(root).ReadHostInterventions("amber", "", 10)
	if err != nil {
		t.Fatalf("read host interventions: %v", err)
	}
	if len(interventions) != 1 {
		t.Fatalf("expected 1 host intervention, got %d", len(interventions))
	}
	if interventions[0].Status != "failed" {
		t.Fatalf("expected failed intervention, got %#v", interventions[0])
	}
}

func TestHostActionServiceRollbackResourceMaintenanceMarksInterventionRolledBack(t *testing.T) {
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
		TicketID:               ticket.ID,
		Resident:               "amber",
		Resource:               "cpu",
		Amount:                 "2",
		Note:                   "Approved for compute burst.",
		Operator:               "chenglin",
		AlsoCreateIntervention: true,
		CreateHostCheckpoint:   true,
	}); err != nil {
		t.Fatalf("plan maintenance: %v", err)
	}
	checkpointName := fake.snapshotName
	if _, err := service.StartResourceMaintenance(ResourceMaintenanceStartInput{
		TicketID:       ticket.ID,
		Resident:       "amber",
		Resource:       "cpu",
		Amount:         "2",
		Note:           "Maintenance window has started.",
		Operator:       "chenglin",
		CheckpointName: checkpointName,
	}); err != nil {
		t.Fatalf("start maintenance: %v", err)
	}

	updated, err := service.RollbackResourceMaintenance(ResourceMaintenanceRollbackInput{
		TicketID:       ticket.ID,
		Resident:       "amber",
		Resource:       "cpu",
		Amount:         "2",
		Note:           "Rolled back to the pre-maintenance checkpoint after validation failure.",
		Close:          true,
		Operator:       "chenglin",
		CheckpointName: checkpointName,
	})
	if err != nil {
		t.Fatalf("rollback maintenance: %v", err)
	}
	last := updated.Replies[len(updated.Replies)-1]
	if !strings.Contains(last.Body, "maintenance_rolled_back=true") {
		t.Fatalf("expected maintenance rolled back marker, got %q", last.Body)
	}
	if !strings.Contains(last.Body, "maintenance_state=rolled_back") {
		t.Fatalf("expected maintenance rolled back state, got %q", last.Body)
	}
	if !refreshed {
		t.Fatalf("expected rollback to refresh inventory snapshot")
	}
	interventions, err := worldstate.New(root).ReadHostInterventions("amber", "", 10)
	if err != nil {
		t.Fatalf("read host interventions: %v", err)
	}
	if len(interventions) != 1 {
		t.Fatalf("expected 1 host intervention, got %d", len(interventions))
	}
	if interventions[0].Status != "rolled_back" {
		t.Fatalf("expected rolled_back intervention, got %#v", interventions[0])
	}
	records := readMaintenanceRunRecords(t, root)
	if len(records) != 3 {
		t.Fatalf("expected planned, in_progress, rolled_back maintenance records, got %#v", records)
	}
	if records[0].State != "planned" || records[1].State != "in_progress" || records[2].State != "rolled_back" {
		t.Fatalf("unexpected maintenance record states: %#v", records)
	}
	if records[2].CheckpointName != checkpointName || !records[2].InventoryRefreshed {
		t.Fatalf("expected rollback record checkpoint and inventory refresh: %#v", records[2])
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

func readMaintenanceRunRecords(t *testing.T, root string) []MaintenanceRunRecord {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "operations", "maintenance-runs-*.jsonl"))
	if err != nil {
		t.Fatalf("glob maintenance records: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one maintenance record file, got %#v", files)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read maintenance records: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	records := make([]MaintenanceRunRecord, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record MaintenanceRunRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode maintenance record %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}
