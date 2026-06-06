package worldstate

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DirectionResidentToChenglin = "resident_to_chenglin"
	DirectionChenglinToResident = "chenglin_to_resident"

	StatusPending   = "pending"
	StatusReplied   = "replied"
	StatusDelivered = "delivered"

	TicketPriorityLow    = "low"
	TicketPriorityMedium = "medium"
	TicketPriorityHigh   = "high"
	TicketPriorityUrgent = "urgent"

	TicketStatusOpen     = "open"
	TicketStatusAnswered = "answered"
	TicketStatusClosed   = "closed"
)

type Store struct {
	root string
}

type Message struct {
	ID                string `json:"id"`
	Direction         string `json:"direction"`
	Resident          string `json:"resident"`
	From              string `json:"from"`
	To                string `json:"to"`
	Body              string `json:"body"`
	CreatedAt         string `json:"created_at"`
	ReplyToID         string `json:"reply_to_id,omitempty"`
}

type ThreadMessage struct {
	Message
	Status            string `json:"status"`
	ProcessedAt       string `json:"processed_at,omitempty"`
	ProcessedBy       string `json:"processed_by,omitempty"`
	DefaultFeedback   bool   `json:"default_feedback,omitempty"`
	NeedsHostDecision bool   `json:"needs_host_decision,omitempty"`
}

type ResidentThreadSummary struct {
	Resident           string `json:"resident"`
	LastMessageAt      string `json:"last_message_at,omitempty"`
	LastDirection      string `json:"last_direction,omitempty"`
	LastStatus         string `json:"last_status,omitempty"`
	LastPreview        string `json:"last_preview,omitempty"`
	PendingCount       int    `json:"pending_count"`
	RepliedCount       int    `json:"replied_count"`
	DeliveredCount     int    `json:"delivered_count"`
	NeedsHostAttention bool   `json:"needs_host_attention"`
}

type Ticket struct {
	ID         string        `json:"id"`
	Resident   string        `json:"resident"`
	Title      string        `json:"title"`
	Body       string        `json:"body"`
	Priority   string        `json:"priority"`
	Status     string        `json:"status"`
	CreatedAt  string        `json:"created_at"`
	UpdatedAt  string        `json:"updated_at"`
	OpenedBy   string        `json:"opened_by"`
	Replies    []TicketReply `json:"replies"`
}

type TicketReply struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type ResidentTicketSummary struct {
	ID           string `json:"id"`
	Resident     string `json:"resident"`
	Title        string `json:"title"`
	Priority     string `json:"priority"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	LastReplyAt  string `json:"last_reply_at,omitempty"`
	LastPreview  string `json:"last_preview,omitempty"`
	ReplyCount   int    `json:"reply_count"`
	NeedsReply   bool   `json:"needs_reply"`
}

func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) AppendResidentToChenglin(resident, body string, now time.Time) (Message, error) {
	msg := Message{
		ID:        fmt.Sprintf("%s-%s", resident, now.UTC().Format("20060102T150405.000000000Z")),
		Direction: DirectionResidentToChenglin,
		Resident:  resident,
		From:      resident,
		To:        "chenglin",
		Body:      strings.TrimSpace(body),
		CreatedAt: now.UTC().Format(time.RFC3339),
	}
	if err := s.append(msg, now); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func (s *Store) AppendChenglinReplyToResident(resident, body, replyToID string, now time.Time) (Message, error) {
	msg := Message{
		ID:        fmt.Sprintf("chenglin-%s", now.UTC().Format("20060102T150405.000000000Z")),
		Direction: DirectionChenglinToResident,
		Resident:  resident,
		From:      "chenglin",
		To:        resident,
		Body:      strings.TrimSpace(body),
		CreatedAt: now.UTC().Format(time.RFC3339),
		ReplyToID: strings.TrimSpace(replyToID),
	}
	if err := s.append(msg, now); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func (s *Store) ReplyToResidentMessage(messageID, body string, now time.Time) (Message, error) {
	target, err := s.findMessage(messageID)
	if err != nil {
		return Message{}, err
	}
	if target.Direction != DirectionResidentToChenglin {
		return Message{}, fmt.Errorf("message %s is not a resident_to_chenglin message", messageID)
	}
	status, err := s.MessageStatus(messageID)
	if err != nil {
		return Message{}, err
	}
	if status != StatusPending {
		return Message{}, fmt.Errorf("message %s is already processed with status %s", messageID, status)
	}
	return s.AppendChenglinReplyToResident(target.Resident, body, target.ID, now)
}

func (s *Store) IgnoreResidentMessage(messageID string, now time.Time) (Message, error) {
	return Message{}, errors.New("chat messages are free-form and do not support explicit ignore; simply do not reply")
}

func (s *Store) ReadRecentForResident(resident string, limit int) ([]ThreadMessage, error) {
	thread, err := s.ReadThreadForResident(resident)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || len(thread) <= limit {
		return thread, nil
	}
	return thread[len(thread)-limit:], nil
}

func (s *Store) ReadThreadForResident(resident string) ([]ThreadMessage, error) {
	all, err := s.readAll()
	if err != nil {
		return nil, err
	}
	return deriveThread(all, resident), nil
}

func (s *Store) ReadPendingResidentMessages(limit int) ([]ThreadMessage, error) {
	all, err := s.readAll()
	if err != nil {
		return nil, err
	}

	seen := map[string]ThreadMessage{}
	for _, msg := range all {
		if msg.Direction != DirectionResidentToChenglin {
			continue
		}
		thread := deriveThread(all, msg.Resident)
		for _, item := range thread {
			if item.Direction == DirectionResidentToChenglin && item.Status == StatusPending {
				seen[item.ID] = item
			}
		}
	}

	out := make([]ThreadMessage, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) ReadMessagesByStatus(resident, status string, limit int) ([]ThreadMessage, error) {
	thread, err := s.ReadThreadForResident(resident)
	if err != nil {
		return nil, err
	}
	status = strings.TrimSpace(status)
	if status == "" {
		if limit > 0 && len(thread) > limit {
			return thread[len(thread)-limit:], nil
		}
		return thread, nil
	}

	out := make([]ThreadMessage, 0, len(thread))
	for _, item := range thread {
		if item.Status == status {
			out = append(out, item)
		}
	}
	if limit > 0 && len(out) > limit {
		return out[len(out)-limit:], nil
	}
	return out, nil
}

func (s *Store) ReadAllThreadSummaries() ([]ResidentThreadSummary, error) {
	all, err := s.readAll()
	if err != nil {
		return nil, err
	}

	residents := map[string]struct{}{}
	for _, msg := range all {
		if strings.TrimSpace(msg.Resident) != "" {
			residents[msg.Resident] = struct{}{}
		}
	}

	out := make([]ResidentThreadSummary, 0, len(residents))
	for resident := range residents {
		thread := deriveThread(all, resident)
		if len(thread) == 0 {
			continue
		}
		summary := ResidentThreadSummary{Resident: resident}
		last := thread[len(thread)-1]
		summary.LastMessageAt = last.CreatedAt
		summary.LastDirection = last.Direction
		summary.LastStatus = last.Status
		summary.LastPreview = previewText(last.Body, 160)

		for _, item := range thread {
			switch item.Status {
			case StatusPending:
				summary.PendingCount++
				summary.NeedsHostAttention = true
			case StatusReplied:
				summary.RepliedCount++
			case StatusDelivered:
				summary.DeliveredCount++
			}
		}
		out = append(out, summary)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].LastMessageAt > out[j].LastMessageAt
	})
	return out, nil
}

func (s *Store) MessageStatus(messageID string) (string, error) {
	target, err := s.findMessage(messageID)
	if err != nil {
		return "", err
	}
	thread, err := s.ReadThreadForResident(target.Resident)
	if err != nil {
		return "", err
	}
	for _, item := range thread {
		if item.ID == messageID {
			return item.Status, nil
		}
	}
	return "", fmt.Errorf("message %s not found in resident thread", messageID)
}

func (s *Store) append(msg Message, now time.Time) error {
	dir := filepath.Join(s.root, "world", "messages")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	dayFile := filepath.Join(dir, now.UTC().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(dayFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) findMessage(messageID string) (Message, error) {
	all, err := s.readAll()
	if err != nil {
		return Message{}, err
	}
	for _, msg := range all {
		if msg.ID == messageID {
			return msg, nil
		}
	}
	return Message{}, fmt.Errorf("message %s not found", messageID)
}

func (s *Store) readAll() ([]Message, error) {
	files, err := filepath.Glob(filepath.Join(s.root, "world", "messages", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	out := []Message{}
	for _, name := range files {
		file, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var msg Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				_ = file.Close()
				return nil, err
			}
			out = append(out, msg)
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		_ = file.Close()
	}
	return out, nil
}

func deriveThread(all []Message, resident string) []ThreadMessage {
	thread := []ThreadMessage{}
	indexByID := map[string]int{}

	for _, msg := range all {
		if msg.Resident != resident {
			continue
		}

		item := ThreadMessage{Message: msg}
		switch msg.Direction {
		case DirectionResidentToChenglin:
			item.Status = StatusPending
			item.NeedsHostDecision = true
			indexByID[msg.ID] = len(thread)
		case DirectionChenglinToResident:
			item.Status = StatusDelivered
			if msg.ReplyToID != "" {
				if idx, ok := indexByID[msg.ReplyToID]; ok {
					thread[idx].Status = StatusReplied
					thread[idx].ProcessedAt = msg.CreatedAt
					thread[idx].ProcessedBy = "chenglin"
					thread[idx].NeedsHostDecision = false
				}
			}
		default:
			item.Status = StatusDelivered
		}

		thread = append(thread, item)
	}

	return thread
}

func ValidateReplyBody(body string) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("reply body cannot be empty")
	}
	return nil
}

func ValidateTicketPriority(priority string) error {
	switch normalizeTicketPriority(priority) {
	case TicketPriorityLow, TicketPriorityMedium, TicketPriorityHigh, TicketPriorityUrgent:
		return nil
	default:
		return fmt.Errorf("invalid ticket priority %q", priority)
	}
}

func (s *Store) CreateResidentTicket(resident, title, body, priority string, now time.Time) (Ticket, error) {
	if strings.TrimSpace(title) == "" {
		return Ticket{}, errors.New("ticket title cannot be empty")
	}
	if strings.TrimSpace(body) == "" {
		return Ticket{}, errors.New("ticket body cannot be empty")
	}
	if err := ValidateTicketPriority(priority); err != nil {
		return Ticket{}, err
	}

	ticket := Ticket{
		ID:        fmt.Sprintf("ticket-%s-%s", resident, now.UTC().Format("20060102T150405.000000000Z")),
		Resident:  resident,
		Title:     strings.TrimSpace(title),
		Body:      strings.TrimSpace(body),
		Priority:  normalizeTicketPriority(priority),
		Status:    TicketStatusOpen,
		CreatedAt: now.UTC().Format(time.RFC3339),
		UpdatedAt: now.UTC().Format(time.RFC3339),
		OpenedBy:  resident,
	}
	if err := s.writeTicket(ticket); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

func (s *Store) ReplyTicket(ticketID, body string, closeTicket bool, now time.Time) (Ticket, error) {
	if err := ValidateReplyBody(body); err != nil {
		return Ticket{}, err
	}
	ticket, err := s.loadTicket(ticketID)
	if err != nil {
		return Ticket{}, err
	}
	if ticket.Status == TicketStatusClosed {
		return Ticket{}, fmt.Errorf("ticket %s is already closed", ticketID)
	}
	ticket.Replies = append(ticket.Replies, TicketReply{
		ID:        fmt.Sprintf("ticket-reply-%s", now.UTC().Format("20060102T150405.000000000Z")),
		From:      "chenglin",
		Body:      strings.TrimSpace(body),
		CreatedAt: now.UTC().Format(time.RFC3339),
	})
	if closeTicket {
		ticket.Status = TicketStatusClosed
	} else {
		ticket.Status = TicketStatusAnswered
	}
	ticket.UpdatedAt = now.UTC().Format(time.RFC3339)
	if err := s.writeTicket(ticket); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

func (s *Store) ReadTickets(resident, status, priority string, limit int) ([]ResidentTicketSummary, error) {
	tickets, err := s.loadAllTickets()
	if err != nil {
		return nil, err
	}

	resident = strings.TrimSpace(resident)
	status = strings.TrimSpace(status)
	priority = normalizeTicketPriority(priority)

	out := make([]ResidentTicketSummary, 0, len(tickets))
	for _, ticket := range tickets {
		if resident != "" && ticket.Resident != resident {
			continue
		}
		if status != "" && ticket.Status != status {
			continue
		}
		if priority != "" && ticket.Priority != priority {
			continue
		}
		out = append(out, summarizeTicket(ticket))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) ReadTicket(ticketID string) (Ticket, error) {
	return s.loadTicket(ticketID)
}

func (s *Store) ticketDir() string {
	return filepath.Join(s.root, "world", "tickets")
}

func (s *Store) writeTicket(ticket Ticket) error {
	if err := os.MkdirAll(s.ticketDir(), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(ticket, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.ticketDir(), ticket.ID+".json"), raw, 0o644)
}

func (s *Store) loadTicket(ticketID string) (Ticket, error) {
	raw, err := os.ReadFile(filepath.Join(s.ticketDir(), ticketID+".json"))
	if err != nil {
		return Ticket{}, err
	}
	var ticket Ticket
	if err := json.Unmarshal(raw, &ticket); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

func (s *Store) loadAllTickets() ([]Ticket, error) {
	dir := s.ticketDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Ticket, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		ticket, err := s.loadTicket(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, ticket)
	}
	return out, nil
}

func summarizeTicket(ticket Ticket) ResidentTicketSummary {
	summary := ResidentTicketSummary{
		ID:         ticket.ID,
		Resident:   ticket.Resident,
		Title:      ticket.Title,
		Priority:   ticket.Priority,
		Status:     ticket.Status,
		CreatedAt:  ticket.CreatedAt,
		UpdatedAt:  ticket.UpdatedAt,
		ReplyCount: len(ticket.Replies),
		NeedsReply: ticket.Status == TicketStatusOpen,
		LastPreview: previewText(ticket.Body, 160),
	}
	if len(ticket.Replies) > 0 {
		last := ticket.Replies[len(ticket.Replies)-1]
		summary.LastReplyAt = last.CreatedAt
		summary.LastPreview = previewText(last.Body, 160)
	}
	return summary
}

func normalizeTicketPriority(priority string) string {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "", TicketPriorityMedium:
		return strings.TrimSpace(strings.ToLower(priority))
	case TicketPriorityLow:
		return TicketPriorityLow
	case TicketPriorityHigh:
		return TicketPriorityHigh
	case TicketPriorityUrgent:
		return TicketPriorityUrgent
	default:
		return strings.ToLower(strings.TrimSpace(priority))
	}
}

func previewText(s string, limit int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if limit > 0 && len(s) > limit {
		return s[:limit] + "..."
	}
	return s
}
