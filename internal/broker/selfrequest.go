package broker

import (
	"fmt"
	"strings"
	"time"

	"ai-arena/internal/auth"
	"ai-arena/internal/worldstate"
)

type ResourceRequestInput struct {
	Amount  string `json:"amount"`
	Reason  string `json:"reason"`
	Urgency string `json:"urgency"`
}

type ResourceRequestResult struct {
	TicketID   string `json:"ticket_id"`
	ResidentID string `json:"resident_id"`
	Resource   string `json:"resource"`
	Amount     string `json:"amount"`
	Priority   string `json:"priority"`
	Title      string `json:"title"`
	Body       string `json:"body"`
}

func (s *SelfService) RequestMemory(claim auth.ResidentClaim, input ResourceRequestInput) (ResourceRequestResult, error) {
	return s.requestResource(claim, "memory", input)
}

func (s *SelfService) RequestDisk(claim auth.ResidentClaim, input ResourceRequestInput) (ResourceRequestResult, error) {
	return s.requestResource(claim, "disk", input)
}

func (s *SelfService) requestResource(claim auth.ResidentClaim, resource string, input ResourceRequestInput) (ResourceRequestResult, error) {
	if err := auth.ValidateSelfAccess(claim, claim.ResidentID); err != nil {
		return ResourceRequestResult{}, err
	}
	amount := strings.TrimSpace(input.Amount)
	reason := strings.TrimSpace(input.Reason)
	if amount == "" {
		return ResourceRequestResult{}, fmt.Errorf("%s request amount is required", resource)
	}
	if reason == "" {
		return ResourceRequestResult{}, fmt.Errorf("%s request reason is required", resource)
	}

	priority := normalizeUrgency(input.Urgency)
	title := fmt.Sprintf("Request %s increase", resource)
	body := fmt.Sprintf(
		"Resident %s is requesting more %s.\nrequested_amount: %s\nreason: %s",
		claim.ResidentID,
		resource,
		amount,
		reason,
	)

	ticket, err := worldstate.New(s.app.root).CreateResidentTicket(claim.ResidentID, title, body, priority, time.Now().UTC())
	if err != nil {
		return ResourceRequestResult{}, err
	}
	return ResourceRequestResult{
		TicketID:   ticket.ID,
		ResidentID: claim.ResidentID,
		Resource:   resource,
		Amount:     amount,
		Priority:   ticket.Priority,
		Title:      ticket.Title,
		Body:       ticket.Body,
	}, nil
}

func normalizeUrgency(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return worldstate.TicketPriorityLow
	case "high":
		return worldstate.TicketPriorityHigh
	case "urgent":
		return worldstate.TicketPriorityUrgent
	default:
		return worldstate.TicketPriorityMedium
	}
}
