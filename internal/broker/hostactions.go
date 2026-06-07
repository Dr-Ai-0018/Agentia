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
}

func NewHostActionService(root string) *HostActionService {
	return &HostActionService{
		world:   worldstate.New(root),
		audit:   audit.New(root),
		history: world.New(root),
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

func (s *HostActionService) Ignore(messageID string) (worldstate.Message, error) {
	msg, err := s.world.IgnoreResidentMessage(messageID, time.Now().UTC())
	if err != nil {
		return worldstate.Message{}, err
	}
	_ = s.audit.Write(audit.Event{
		Actor:      "chenglin",
		ResidentID: msg.Resident,
		Kind:       "chat_ignore",
		TargetID:   msg.ID,
		Summary:    fmt.Sprintf("Closed %s chat thread with default feedback", msg.Resident),
		Metadata: map[string]any{
			"reply_to_id": msg.ReplyToID,
		},
	})
	_ = s.history.Write(world.HistoryEntry{
		ResidentID: msg.Resident,
		Kind:       "chat_ignore",
		Summary:    fmt.Sprintf("A %s chat thread was closed without direct reply", msg.Resident),
		Details: map[string]any{
			"reply_to_id": msg.ReplyToID,
			"message_id":  msg.ID,
		},
	})
	return msg, nil
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
