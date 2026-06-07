package newborn

import (
	"fmt"
	"strings"
	"time"

	"ai-arena/internal/worldstate"
)

type WorldBridge struct {
	store *worldstate.Store
}

func NewWorldBridge(root string) *WorldBridge {
	return &WorldBridge{store: worldstate.New(root)}
}

func (w *WorldBridge) RecordResidentMessage(profile ResidentProfile, body string, now time.Time) (string, error) {
	msg, err := w.store.AppendResidentToChenglin(profile.Name, body, now)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("message delivered to Chenglin and recorded in world state:\nmessage_id=%s\nstatus=%s\ncreated_at=%s\nbody=%s", msg.ID, worldstate.StatusPending, msg.CreatedAt, msg.Body), nil
}

func (w *WorldBridge) CreateResidentTicket(profile ResidentProfile, title, body, priority string, now time.Time) (string, error) {
	ticket, err := w.store.CreateResidentTicket(profile.Name, title, body, priority, now)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ticket created in world state:\nticket_id=%s\npriority=%s\nstatus=%s\ncreated_at=%s\ntitle=%s\nbody=%s", ticket.ID, ticket.Priority, ticket.Status, ticket.CreatedAt, ticket.Title, ticket.Body), nil
}

func (w *WorldBridge) BuildResidentWorldContext(profile ResidentProfile, limit int) string {
	messages, err := w.store.ReadRecentForResident(profile.Name, limit)
	header := []string{
		"chat_mode: free-form and asynchronous",
		"chat_rule: you may send multiple chat messages without waiting",
		"chat_rule: Chenglin may reply later, reply multiple times, or not reply at all",
		"ticket_mode: formal host-decision objects with priority and explicit resolution state",
		"ticket_rule: use chat for ordinary conversation; use tickets for requests that require a clear host decision",
	}
	if err != nil || len(messages) == 0 {
		header = append(header, "recent_chat: none recorded")
		if ticketBlock := w.buildResidentTicketBlock(profile, 6); ticketBlock != "" {
			header = append(header, ticketBlock)
		}
		return strings.Join(header, "\n")
	}

	lines := append(header, "recent_chat:")
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		suffix := ""
		if msg.ReplyToID != "" {
			suffix = fmt.Sprintf(" reply_to=%s", msg.ReplyToID)
		}
		if msg.DefaultFeedback {
			suffix += " default_feedback=true"
		}
		lines = append(lines, fmt.Sprintf("- [%s] status=%s %s -> %s%s: %s", msg.CreatedAt, msg.Status, msg.From, msg.To, suffix, oneLine(msg.Body)))
	}
	if ticketBlock := w.buildResidentTicketBlock(profile, 6); ticketBlock != "" {
		lines = append(lines, ticketBlock)
	}
	return strings.Join(lines, "\n")
}

func (w *WorldBridge) buildResidentTicketBlock(profile ResidentProfile, limit int) string {
	tickets, err := w.store.ReadTickets(profile.Name, "", "", limit)
	if err != nil || len(tickets) == 0 {
		return "recent_tickets: none recorded"
	}

	lines := []string{"recent_tickets:"}
	for _, ticket := range tickets {
		lines = append(lines, fmt.Sprintf("- [%s] ticket=%s priority=%s status=%s needs_reply=%t title=%s preview=%s",
			ticket.UpdatedAt,
			ticket.ID,
			ticket.Priority,
			ticket.Status,
			ticket.NeedsReply,
			ticket.Title,
			oneLine(ticket.LastPreview),
		))
	}
	return strings.Join(lines, "\n")
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 280 {
		return s[:280] + "..."
	}
	return s
}
