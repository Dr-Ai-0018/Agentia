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
	return fmt.Sprintf("消息已送达程林，并记录到 world state:\nmessage_id=%s\nstatus=%s\ncreated_at=%s\nbody=%s", msg.ID, worldstate.StatusPending, msg.CreatedAt, msg.Body), nil
}

func (w *WorldBridge) CreateResidentTicket(profile ResidentProfile, title, body, priority string, now time.Time) (string, error) {
	ticket, err := w.store.CreateResidentTicket(profile.Name, title, body, priority, now)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ticket 已创建并记录到 world state:\nticket_id=%s\npriority=%s\nstatus=%s\ncreated_at=%s\ntitle=%s\nbody=%s", ticket.ID, ticket.Priority, ticket.Status, ticket.CreatedAt, ticket.Title, ticket.Body), nil
}

func (w *WorldBridge) BuildResidentWorldContext(profile ResidentProfile, limit int) string {
	return w.BuildResidentWorldView(profile, limit).RenderedChat
}

func (w *WorldBridge) BuildResidentWorldView(profile ResidentProfile, limit int) ResidentWorldView {
	messages, err := w.store.ReadRecentForResident(profile.Name, limit)
	header := []string{
		"chat_mode: 自由、异步",
		"chat_rule: 你可以不等待回复就发送多条聊天消息",
		"chat_rule: 程林可能稍后回复、多次回复，也可能不回复",
		"chat_rule: pending 只是异步消息状态，不是暂停、等待命令或停止探索的理由",
		"relationship_rule: 你和程林不是主人/副手或雇佣关系；普通聊天、沉默、回应和继续做自己的事都可以自然共存",
		"ticket_mode: 带 priority 和明确 resolution state 的正式宿主决策对象",
		"ticket_rule: 普通对话走 chat；需要明确宿主决策的请求走 ticket",
	}
	ticketBlock, freshTicketItems := w.buildResidentTicketBlock(profile, 6)
	interventionBlock, freshInterventionItems := w.buildResidentInterventionBlock(profile, 6)

	if err != nil || len(messages) == 0 {
		header = append(header, "recent_chat: 尚无记录")
		if ticketBlock != "" {
			header = append(header, ticketBlock)
		}
		if interventionBlock != "" {
			header = append(header, interventionBlock)
		}
		return ResidentWorldView{
			RenderedChat:        strings.Join(header, "\n"),
			FreshDeliveredItems: append(freshTicketItems, freshInterventionItems...),
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
		lines = append(lines, fmt.Sprintf("- [%s] status=%s %s -> %s%s: %s", msg.CreatedAt, msg.Status, msg.From, msg.To, suffix, oneLine(msg.Body)))
	}
	if ticketBlock != "" {
		lines = append(lines, ticketBlock)
	}
	if interventionBlock != "" {
		lines = append(lines, interventionBlock)
	}
	return ResidentWorldView{
		RenderedChat:        strings.Join(lines, "\n"),
		FreshDeliveredItems: append(append(freshDelivered, freshTicketItems...), freshInterventionItems...),
	}
}

func (w *WorldBridge) buildResidentTicketBlock(profile ResidentProfile, limit int) (string, []string) {
	tickets, fresh, err := w.store.ConsumeFreshTicketUpdates(profile.Name, limit)
	if err != nil || len(tickets) == 0 {
		return "recent_tickets: 尚无记录", nil
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

func (w *WorldBridge) buildResidentInterventionBlock(profile ResidentProfile, limit int) (string, []string) {
	items, fresh, err := w.store.ConsumeFreshHostInterventions(profile.Name, limit)
	if err != nil || len(items) == 0 {
		return "recent_host_interventions: 尚无记录", nil
	}
	lines := []string{"recent_host_interventions:"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- [%s] intervention=%s kind=%s status=%s title=%s preview=%s",
			item.UpdatedAt,
			item.ID,
			item.Kind,
			item.Status,
			oneLine(item.Title),
			oneLine(item.LastPreview),
		))
	}
	freshLines := make([]string, 0, len(fresh))
	for _, item := range fresh {
		freshLines = append(freshLines, fmt.Sprintf("host_intervention_update intervention=%s kind=%s status=%s title=%s preview=%s",
			item.ID,
			item.Kind,
			item.Status,
			oneLine(item.Title),
			oneLine(item.LastPreview),
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
