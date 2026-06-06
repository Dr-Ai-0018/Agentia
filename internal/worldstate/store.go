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
	DirectionSystemToResident   = "system_to_resident"

	StatusPending   = "pending"
	StatusReplied   = "replied"
	StatusIgnored   = "ignored"
	StatusDelivered = "delivered"
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
	AutoFeedbackForID string `json:"auto_feedback_for_id,omitempty"`
}

type ThreadMessage struct {
	Message
	Status            string `json:"status"`
	ProcessedAt       string `json:"processed_at,omitempty"`
	ProcessedBy       string `json:"processed_by,omitempty"`
	DefaultFeedback   bool   `json:"default_feedback,omitempty"`
	NeedsHostDecision bool   `json:"needs_host_decision,omitempty"`
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

	body := fmt.Sprintf(
		"Your message from %s was received by the world, but Chenglin chose not to reply right now. Treat that as real signal, not as a frozen state. Continue acting from your current evidence instead of waiting.",
		target.CreatedAt,
	)
	msg := Message{
		ID:                fmt.Sprintf("system-%s", now.UTC().Format("20060102T150405.000000000Z")),
		Direction:         DirectionSystemToResident,
		Resident:          target.Resident,
		From:              "system",
		To:                target.Resident,
		Body:              body,
		CreatedAt:         now.UTC().Format(time.RFC3339),
		ReplyToID:         target.ID,
		AutoFeedbackForID: target.ID,
	}
	if err := s.append(msg, now); err != nil {
		return Message{}, err
	}
	return msg, nil
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
		case DirectionSystemToResident:
			item.Status = StatusDelivered
			if msg.AutoFeedbackForID != "" {
				item.DefaultFeedback = true
			}
			targetID := msg.AutoFeedbackForID
			if targetID == "" {
				targetID = msg.ReplyToID
			}
			if targetID != "" {
				if idx, ok := indexByID[targetID]; ok {
					thread[idx].Status = StatusIgnored
					thread[idx].ProcessedAt = msg.CreatedAt
					thread[idx].ProcessedBy = "system"
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
