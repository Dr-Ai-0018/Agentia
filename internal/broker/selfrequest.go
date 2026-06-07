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

type SubmissionInput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Details string `json:"details"`
}

type SubmissionResult struct {
	MessageID  string `json:"message_id"`
	ResidentID string `json:"resident_id"`
	Title      string `json:"title"`
	Body       string `json:"body"`
}

var resourceRequestTitles = map[string]string{
	"cpu":        "Request CPU increase",
	"memory":     "Request memory increase",
	"disk":       "Request disk increase",
	"gpu-time":   "Request GPU time",
	"vps-access": "Request VPS access",
}

var resourceRequestBodies = map[string]string{
	"cpu":        "Resident %s is requesting more cpu.\nrequested_amount: %s\nreason: %s",
	"memory":     "Resident %s is requesting more memory.\nrequested_amount: %s\nreason: %s",
	"disk":       "Resident %s is requesting more disk.\nrequested_amount: %s\nreason: %s",
	"gpu-time":   "Resident %s is requesting gpu time.\nrequested_amount: %s\nreason: %s",
	"vps-access": "Resident %s is requesting vps access.\nrequested_amount: %s\nreason: %s",
}

func (s *SelfService) RequestCPU(claim auth.ResidentClaim, input ResourceRequestInput) (ResourceRequestResult, error) {
	return s.requestResource(claim, "cpu", input)
}

func (s *SelfService) RequestMemory(claim auth.ResidentClaim, input ResourceRequestInput) (ResourceRequestResult, error) {
	return s.requestResource(claim, "memory", input)
}

func (s *SelfService) RequestDisk(claim auth.ResidentClaim, input ResourceRequestInput) (ResourceRequestResult, error) {
	return s.requestResource(claim, "disk", input)
}

func (s *SelfService) RequestGPUTime(claim auth.ResidentClaim, input ResourceRequestInput) (ResourceRequestResult, error) {
	return s.requestResource(claim, "gpu-time", input)
}

func (s *SelfService) RequestVPSAccess(claim auth.ResidentClaim, input ResourceRequestInput) (ResourceRequestResult, error) {
	return s.requestResource(claim, "vps-access", input)
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
	bodyFormat, ok := resourceRequestBodies[resource]
	if !ok {
		return ResourceRequestResult{}, fmt.Errorf("unsupported resource request type: %s", resource)
	}

	priority := normalizeUrgency(input.Urgency)
	title := resourceRequestTitles[resource]
	body := fmt.Sprintf(
		bodyFormat,
		claim.ResidentID,
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

func (s *SelfService) SubmitResult(claim auth.ResidentClaim, input SubmissionInput) (SubmissionResult, error) {
	if err := auth.ValidateSelfAccess(claim, claim.ResidentID); err != nil {
		return SubmissionResult{}, err
	}
	title := strings.TrimSpace(input.Title)
	summary := strings.TrimSpace(input.Summary)
	details := strings.TrimSpace(input.Details)
	if title == "" {
		return SubmissionResult{}, fmt.Errorf("result title is required")
	}
	if summary == "" {
		return SubmissionResult{}, fmt.Errorf("result summary is required")
	}

	body := fmt.Sprintf("Result from %s\ntitle: %s\nsummary: %s", claim.ResidentID, title, summary)
	if details != "" {
		body += "\ndetails: " + details
	}

	message, err := worldstate.New(s.app.root).AppendResidentToChenglin(claim.ResidentID, body, time.Now().UTC())
	if err != nil {
		return SubmissionResult{}, err
	}
	return SubmissionResult{
		MessageID:  message.ID,
		ResidentID: claim.ResidentID,
		Title:      title,
		Body:       message.Body,
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
