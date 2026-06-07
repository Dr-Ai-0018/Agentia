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

type ResidentWorldView struct {
	RenderedChat        string
	FreshDeliveredItems []string
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
	return w.BuildResidentWorldView(profile, limit).RenderedChat
}

func (w *WorldBridge) BuildResidentWorldView(profile ResidentProfile, limit int) ResidentWorldView {
	messages, err := w.store.ReadRecentForResident(profile.Name, limit)
	header := []string{
		"chat_mode: free-form and asynchronous",
		"chat_rule: you may send multiple chat messages without waiting",
		"chat_rule: Chenglin may reply later, reply multiple times, or not reply at all",
		"ticket_mode: formal host-decision objects with priority and explicit resolution state",
		"ticket_rule: use chat for ordinary conversation; use tickets for requests that require a clear host decision",
	}
	ticketBlock, freshTicketItems := w.buildResidentTicketBlock(profile, 6)

	if err != nil || len(messages) == 0 {
		header = append(header, "recent_chat: none recorded")
		if ticketBlock != "" {
			header = append(header, ticketBlock)
		}
		return ResidentWorldView{
			RenderedChat:        strings.Join(header, "\n"),
			FreshDeliveredItems: freshTicketItems,
		}
	}

	unreadIDs := []string{}
	freshDelivered := []string{}
	for _, msg := range messages {
		if msg.Direction == worldstate.DirectionChenglinToResident && strings.TrimSpace(msg.ReadAt) == "" {
			unreadIDs = append(unreadIDs, msg.ID)
			freshDelivered = append(freshDelivered, fmt.Sprintf("[%s] %s", msg.CreatedAt, oneLine(msg.Body)))
		}
	}
	if len(unreadIDs) > 0 {
		_ = w.store.MarkResidentMessagesRead(profile.Name, unreadIDs, time.Now().UTC())
		for i := range messages {
			for _, id := range unreadIDs {
				if messages[i].ID == id {
					messages[i].ReadAt = time.Now().UTC().Format(time.RFC3339)
				}
			}
		}
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
	if ticketBlock != "" {
		lines = append(lines, ticketBlock)
	}
	return ResidentWorldView{
		RenderedChat:        strings.Join(lines, "\n"),
		FreshDeliveredItems: append(freshDelivered, freshTicketItems...),
	}
}

func (w *WorldBridge) buildResidentTicketBlock(profile ResidentProfile, limit int) (string, []string) {
	tickets, fresh, err := w.store.ConsumeFreshTicketUpdates(profile.Name, limit)
	if err != nil || len(tickets) == 0 {
		return "recent_tickets: none recorded", nil
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
	freshLines := make([]string, 0, len(fresh))
	for _, ticket := range fresh {
		freshLines = append(freshLines, fmt.Sprintf("ticket_update ticket=%s status=%s priority=%s title=%s preview=%s",
			ticket.ID,
			ticket.Status,
			ticket.Priority,
			oneLine(ticket.Title),
			oneLine(ticket.LastPreview),
		))
	}
	return strings.Join(lines, "\n"), freshLines
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
