package broker

import (
	"fmt"
	"strings"
	"time"

	"ai-arena/internal/audit"
	"ai-arena/internal/world"
	"ai-arena/internal/worldstate"
)

type HostActionService struct {
	world   *worldstate.Store
	audit   *audit.Logger
	history *world.History
	app     *App
	machine MachineControl
}

type ResourceSettlementInput struct {
	TicketID  string `json:"ticket_id"`
	Resource  string `json:"resource"`
	Amount    string `json:"amount"`
	Decision  string `json:"decision"`
	Note      string `json:"note"`
	Close     bool   `json:"close"`
}

type ResourceMaintenancePlanInput struct {
	TicketID                string `json:"ticket_id"`
	Resident                string `json:"resident"`
	Resource                string `json:"resource"`
	Amount                  string `json:"amount"`
	Note                    string `json:"note"`
	Window                  string `json:"window"`
	Operator                string `json:"operator"`
	AlsoCreateIntervention  bool   `json:"also_create_intervention"`
	CreateHostCheckpoint    bool   `json:"create_host_checkpoint"`
}

type ResourceMaintenanceCompleteInput struct {
	TicketID            string `json:"ticket_id"`
	Resident            string `json:"resident"`
	Resource            string `json:"resource"`
	Amount              string `json:"amount"`
	Note                string `json:"note"`
	Close               bool   `json:"close"`
	Operator            string `json:"operator"`
	CheckpointName      string `json:"checkpoint_name"`
}

type ResourceMaintenanceStartInput struct {
	TicketID       string `json:"ticket_id"`
	Resident       string `json:"resident"`
	Resource       string `json:"resource"`
	Amount         string `json:"amount"`
	Note           string `json:"note"`
	Operator       string `json:"operator"`
	CheckpointName string `json:"checkpoint_name"`
}

type HostInterventionInput struct {
	Resident string `json:"resident"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Operator string `json:"operator"`
}

func NewHostActionService(root string) *HostActionService {
	return &HostActionService{
		world:   worldstate.New(root),
		audit:   audit.New(root),
		history: world.New(root),
		app:     New(root),
		machine: NewIncusMachineControl(),
	}
}

func (s *HostActionService) Reply(messageID, body string) (worldstate.Message, error) {
	msg, err := s.world.ReplyToResidentMessage(messageID, body, time.Now().UTC())
	if err != nil {
		return worldstate.Message{}, err
	}
	_ = s.audit.Write(audit.Event{
		Actor:      "chenglin",
		ResidentID: msg.Resident,
		Kind:       "chat_reply",
		TargetID:   msg.ID,
		Summary:    fmt.Sprintf("Replied to %s chat thread", msg.Resident),
		Metadata: map[string]any{
			"reply_to_id": msg.ReplyToID,
		},
	})
	_ = s.history.Write(world.HistoryEntry{
		ResidentID: msg.Resident,
		Kind:       "chat_reply",
		Summary:    fmt.Sprintf("Chenglin replied to %s", msg.Resident),
		Details: map[string]any{
			"reply_to_id": msg.ReplyToID,
			"message_id":  msg.ID,
		},
	})
	return msg, nil
}

func (s *HostActionService) CreateHostIntervention(input HostInterventionInput) (worldstate.HostIntervention, error) {
	item, err := s.world.CreateHostIntervention(input.Resident, input.Kind, input.Title, input.Body, input.Operator, time.Now().UTC())
	if err != nil {
		return worldstate.HostIntervention{}, err
	}
	_ = s.audit.Write(audit.Event{
		Actor:      defaultMaintenanceOperator(input.Operator),
		ResidentID: item.Resident,
		Kind:       "host_intervention_create",
		TargetID:   item.ID,
		Summary:    fmt.Sprintf("Created host intervention %s for %s", item.Kind, item.Resident),
		Metadata: map[string]any{
			"kind":     item.Kind,
			"title":    item.Title,
			"status":   item.Status,
			"operator": item.Operator,
		},
	})
	_ = s.history.Write(world.HistoryEntry{
		ResidentID: item.Resident,
		Kind:       "host_intervention_create",
		Summary:    fmt.Sprintf("Chenglin created %s intervention for %s", item.Kind, item.Resident),
		Details: map[string]any{
			"intervention_id": item.ID,
			"kind":            item.Kind,
			"title":           item.Title,
			"status":          item.Status,
			"operator":        item.Operator,
		},
	})
	return item, nil
}

func (s *HostActionService) ReplyTicket(ticketID, body string, closeTicket bool) (worldstate.Ticket, error) {
	ticket, err := s.world.ReplyTicket(ticketID, body, closeTicket, time.Now().UTC())
	if err != nil {
		return worldstate.Ticket{}, err
	}
	kind := "ticket_reply"
	if closeTicket || strings.EqualFold(ticket.Status, worldstate.TicketStatusClosed) {
		kind = "ticket_close"
	}
	_ = s.audit.Write(audit.Event{
		Actor:      "chenglin",
		ResidentID: ticket.Resident,
		Kind:       kind,
		TargetID:   ticket.ID,
		Summary:    fmt.Sprintf("Processed ticket %s for %s", ticket.ID, ticket.Resident),
		Metadata: map[string]any{
			"status":   ticket.Status,
			"priority": ticket.Priority,
		},
	})
	_ = s.history.Write(world.HistoryEntry{
		ResidentID: ticket.Resident,
		Kind:       kind,
		Summary:    fmt.Sprintf("Chenglin processed %s ticket", ticket.Resident),
		Details: map[string]any{
			"ticket_id": ticket.ID,
			"status":    ticket.Status,
			"priority":  ticket.Priority,
		},
	})
	return ticket, nil
}

func (s *HostActionService) SettleResourceTicket(input ResourceSettlementInput) (worldstate.Ticket, error) {
	ticketID := strings.TrimSpace(input.TicketID)
	resource := strings.TrimSpace(input.Resource)
	amount := strings.TrimSpace(input.Amount)
	decision := normalizeSettlementDecision(input.Decision)
	note := strings.TrimSpace(input.Note)
	if ticketID == "" {
		return worldstate.Ticket{}, fmt.Errorf("ticket id is required")
	}
	if resource == "" {
		return worldstate.Ticket{}, fmt.Errorf("resource is required")
	}
	if amount == "" {
		return worldstate.Ticket{}, fmt.Errorf("amount is required")
	}
	if decision == "" {
		return worldstate.Ticket{}, fmt.Errorf("decision is required")
	}

	lines := []string{
		fmt.Sprintf("resource_settlement decision=%s", decision),
		fmt.Sprintf("resource=%s", resource),
		fmt.Sprintf("amount=%s", amount),
	}
	if note != "" {
		lines = append(lines, "note="+note)
	}
	body := strings.Join(lines, "\n")

	ticket, err := s.world.ReplyTicket(ticketID, body, input.Close, time.Now().UTC())
	if err != nil {
		return worldstate.Ticket{}, err
	}

	kind := "resource_settlement"
	if input.Close || strings.EqualFold(ticket.Status, worldstate.TicketStatusClosed) {
		kind = "resource_settlement_close"
	}
	summary := fmt.Sprintf("Settled %s resource ticket for %s (%s %s)", resource, ticket.Resident, decision, amount)
	metadata := map[string]any{
		"resource": resource,
		"amount":   amount,
		"decision": decision,
		"status":   ticket.Status,
	}
	if note != "" {
		metadata["note"] = note
	}
	for k, v := range parseMaintenanceMetadata(note) {
		metadata[k] = v
	}
	_ = s.audit.Write(audit.Event{
		Actor:      "chenglin",
		ResidentID: ticket.Resident,
		Kind:       kind,
		TargetID:   ticket.ID,
		Summary:    summary,
		Metadata:   metadata,
	})
	_ = s.history.Write(world.HistoryEntry{
		ResidentID: ticket.Resident,
		Kind:       kind,
		Summary:    summary,
		Details: map[string]any{
			"ticket_id": ticket.ID,
			"resource":  resource,
			"amount":    amount,
			"decision":  decision,
			"status":    ticket.Status,
			"note":      note,
		},
	})
	return ticket, nil
}

func normalizeSettlementDecision(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approved", "approve":
		return "approved"
	case "rejected", "reject":
		return "rejected"
	case "deferred", "defer", "later":
		return "deferred"
	default:
		return ""
	}
}

func normalizeResource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "cpu", "vcpu":
		return "cpu"
	case "memory", "ram":
		return "memory"
	case "disk", "storage":
		return "disk"
	default:
		return ""
	}
}

func buildMaintenanceApprovalNote(note, window, operator string) string {
	return buildMaintenanceApprovalNoteWithCheckpoint(note, window, operator, "")
}

func buildMaintenanceApprovalNoteWithCheckpoint(note, window, operator, checkpointName string) string {
	lines := []string{}
	if trimmed := strings.TrimSpace(note); trimmed != "" {
		lines = append(lines, trimmed)
	}
	lines = append(lines,
		"approved_for_maintenance=true",
		"maintenance_action=host_planned_stop_change_start",
		fmt.Sprintf("maintenance_window=%s", defaultMaintenanceWindow(window)),
		fmt.Sprintf("operator=%s", defaultMaintenanceOperator(operator)),
		"resident_expectation=normal use can continue until maintenance window; VM will reboot automatically during the approved maintenance change; a follow-up notice will be sent after completion.",
	)
	if checkpointName != "" {
		lines = append(lines, fmt.Sprintf("maintenance_checkpoint=%s", checkpointName))
	}
	return strings.Join(lines, "\n")
}

func buildMaintenanceInterventionTitle(resource, amount string) string {
	return fmt.Sprintf("Planned %s maintenance (%s)", resource, amount)
}

func buildMaintenanceCompletionNote(note string) string {
	return buildMaintenanceCompletionNoteWithOperator(note, "")
}

func buildMaintenanceCompletionNoteWithOperator(note, operator string) string {
	return buildMaintenanceCompletionNoteWithCheckpoint(note, operator, "")
}

func buildMaintenanceCompletionNoteWithCheckpoint(note, operator, checkpointName string) string {
	lines := []string{}
	if trimmed := strings.TrimSpace(note); trimmed != "" {
		lines = append(lines, trimmed)
	}
	lines = append(lines,
		"maintenance_completed=true",
		"maintenance_result=host_stop_change_start_finished",
		fmt.Sprintf("operator=%s", defaultMaintenanceOperator(operator)),
		"resident_expectation=approved resource change has been applied; VM service has been brought back after maintenance.",
	)
	if checkpointName != "" {
		lines = append(lines, fmt.Sprintf("maintenance_checkpoint=%s", checkpointName))
	}
	return strings.Join(lines, "\n")
}

func buildMaintenanceStartNote(note, operator, checkpointName string) string {
	lines := []string{}
	if trimmed := strings.TrimSpace(note); trimmed != "" {
		lines = append(lines, trimmed)
	}
	lines = append(lines,
		"maintenance_started=true",
		"maintenance_state=in_progress",
		fmt.Sprintf("operator=%s", defaultMaintenanceOperator(operator)),
		"resident_expectation=approved maintenance is now underway; temporary unavailability or reboot may occur until completion notice is sent.",
	)
	if checkpointName != "" {
		lines = append(lines, fmt.Sprintf("maintenance_checkpoint=%s", checkpointName))
	}
	return strings.Join(lines, "\n")
}

func defaultMaintenanceWindow(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "pending_schedule"
	}
	return value
}

func defaultMaintenanceOperator(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "chenglin"
	}
	return value
}

func (s *HostActionService) PlanResourceMaintenance(input ResourceMaintenancePlanInput) (worldstate.Ticket, error) {
	ticketID := strings.TrimSpace(input.TicketID)
	residentID := strings.TrimSpace(input.Resident)
	resource := normalizeResource(input.Resource)
	amount := strings.TrimSpace(input.Amount)
	if ticketID == "" {
		return worldstate.Ticket{}, fmt.Errorf("ticket id is required")
	}
	if residentID == "" {
		return worldstate.Ticket{}, fmt.Errorf("resident id is required")
	}
	if resource == "" {
		return worldstate.Ticket{}, fmt.Errorf("resource is required")
	}
	if amount == "" {
		return worldstate.Ticket{}, fmt.Errorf("amount is required")
	}
	if _, ok := s.app.Binding(residentID); !ok {
		return worldstate.Ticket{}, fmt.Errorf("unknown resident binding: %s", residentID)
	}
	checkpointName := ""
	if input.CreateHostCheckpoint {
		created, err := s.CreateHostCheckpoint(residentID, input.Operator, time.Now().UTC())
		if err != nil {
			return worldstate.Ticket{}, err
		}
		checkpointName = created.Name
	}
	approvalNote := buildMaintenanceApprovalNoteWithCheckpoint(input.Note, input.Window, input.Operator, checkpointName)
	ticket, err := s.SettleResourceTicket(ResourceSettlementInput{
		TicketID: ticketID,
		Resource: resource,
		Amount:   amount,
		Decision: "approved",
		Note:     approvalNote,
		Close:    false,
	})
	if err != nil {
		return worldstate.Ticket{}, err
	}
	if input.AlsoCreateIntervention {
		if _, err := s.CreateHostIntervention(HostInterventionInput{
			Resident: residentID,
			Kind:     "maintenance",
			Title:    buildMaintenanceInterventionTitle(resource, amount),
			Body:     approvalNote,
			Operator: input.Operator,
		}); err != nil {
			return worldstate.Ticket{}, err
		}
	}
	return ticket, nil
}

func (s *HostActionService) StartResourceMaintenance(input ResourceMaintenanceStartInput) (worldstate.Ticket, error) {
	ticketID := strings.TrimSpace(input.TicketID)
	residentID := strings.TrimSpace(input.Resident)
	resource := normalizeResource(input.Resource)
	amount := strings.TrimSpace(input.Amount)
	if ticketID == "" {
		return worldstate.Ticket{}, fmt.Errorf("ticket id is required")
	}
	if residentID == "" {
		return worldstate.Ticket{}, fmt.Errorf("resident id is required")
	}
	if resource == "" {
		return worldstate.Ticket{}, fmt.Errorf("resource is required")
	}
	if amount == "" {
		return worldstate.Ticket{}, fmt.Errorf("amount is required")
	}
	if _, ok := s.app.Binding(residentID); !ok {
		return worldstate.Ticket{}, fmt.Errorf("unknown resident binding: %s", residentID)
	}
	startNote := buildMaintenanceStartNote(input.Note, input.Operator, strings.TrimSpace(input.CheckpointName))
	ticket, err := s.SettleResourceTicket(ResourceSettlementInput{
		TicketID: ticketID,
		Resource: resource,
		Amount:   amount,
		Decision: "approved",
		Note:     startNote,
		Close:    false,
	})
	if err != nil {
		return worldstate.Ticket{}, err
	}
	if _, _, err := s.world.UpdateLatestOpenHostIntervention(
		residentID,
		"maintenance",
		buildMaintenanceInterventionTitle(resource, amount),
		"in_progress",
		startNote,
		input.Operator,
		time.Now().UTC(),
	); err != nil {
		return worldstate.Ticket{}, err
	}
	return ticket, nil
}

func (s *HostActionService) CompleteResourceMaintenance(input ResourceMaintenanceCompleteInput) (worldstate.Ticket, error) {
	ticketID := strings.TrimSpace(input.TicketID)
	residentID := strings.TrimSpace(input.Resident)
	resource := normalizeResource(input.Resource)
	amount := strings.TrimSpace(input.Amount)
	if ticketID == "" {
		return worldstate.Ticket{}, fmt.Errorf("ticket id is required")
	}
	if residentID == "" {
		return worldstate.Ticket{}, fmt.Errorf("resident id is required")
	}
	if resource == "" {
		return worldstate.Ticket{}, fmt.Errorf("resource is required")
	}
	if amount == "" {
		return worldstate.Ticket{}, fmt.Errorf("amount is required")
	}
	if _, ok := s.app.Binding(residentID); !ok {
		return worldstate.Ticket{}, fmt.Errorf("unknown resident binding: %s", residentID)
	}
	completionNote := buildMaintenanceCompletionNoteWithCheckpoint(input.Note, input.Operator, strings.TrimSpace(input.CheckpointName))
	ticket, err := s.SettleResourceTicket(ResourceSettlementInput{
		TicketID: ticketID,
		Resource: resource,
		Amount:   amount,
		Decision: "approved",
		Note:     completionNote,
		Close:    input.Close,
	})
	if err != nil {
		return worldstate.Ticket{}, err
	}
	if _, _, err := s.world.ResolveLatestOpenHostIntervention(
		residentID,
		"maintenance",
		buildMaintenanceInterventionTitle(resource, amount),
		completionNote,
		input.Operator,
		time.Now().UTC(),
	); err != nil {
		return worldstate.Ticket{}, err
	}
	if _, _, err := s.app.RefreshInventorySnapshot(time.Now().UTC()); err != nil {
		return worldstate.Ticket{}, fmt.Errorf("refresh inventory snapshot after maintenance: %w", err)
	}
	return ticket, nil
}

func (s *HostActionService) ApplyMemoryAdjustment(ticketID, residentID string, memoryMiB int64, note string) (worldstate.Ticket, error) {
	if memoryMiB <= 0 {
		return worldstate.Ticket{}, fmt.Errorf("memory MiB must be positive")
	}
	return s.PlanResourceMaintenance(ResourceMaintenancePlanInput{
		TicketID:               ticketID,
		Resident:               residentID,
		Resource:               "memory",
		Amount:                 fmt.Sprintf("%dMiB", memoryMiB),
		Note:                   note,
		AlsoCreateIntervention: true,
		CreateHostCheckpoint:   true,
	})
}

func (s *HostActionService) ApplyCPUAdjustment(ticketID, residentID string, vcpu int64, note string) (worldstate.Ticket, error) {
	if vcpu <= 0 {
		return worldstate.Ticket{}, fmt.Errorf("vcpu must be positive")
	}
	return s.PlanResourceMaintenance(ResourceMaintenancePlanInput{
		TicketID:               ticketID,
		Resident:               residentID,
		Resource:               "cpu",
		Amount:                 fmt.Sprintf("%d", vcpu),
		Note:                   note,
		AlsoCreateIntervention: true,
		CreateHostCheckpoint:   true,
	})
}

func (s *HostActionService) ApplyDiskAdjustment(ticketID, residentID string, diskGiB int64, note string) (worldstate.Ticket, error) {
	if diskGiB <= 0 {
		return worldstate.Ticket{}, fmt.Errorf("disk GiB must be positive")
	}
	return s.PlanResourceMaintenance(ResourceMaintenancePlanInput{
		TicketID:               ticketID,
		Resident:               residentID,
		Resource:               "disk",
		Amount:                 fmt.Sprintf("%dGiB", diskGiB),
		Note:                   note,
		AlsoCreateIntervention: true,
		CreateHostCheckpoint:   true,
	})
}

func parseMaintenanceMetadata(note string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(note), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "approved_for_maintenance", "maintenance_action", "maintenance_window", "maintenance_completed", "maintenance_result", "operator", "maintenance_checkpoint":
			out[key] = value
		}
	}
	return out
}
