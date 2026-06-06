package worldstate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Store struct {
	root string
}

type Message struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Resident  string `json:"resident"`
	From      string `json:"from"`
	To        string `json:"to"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) AppendResidentToChenglin(resident, body string, now time.Time) (Message, error) {
	msg := Message{
		ID:        fmt.Sprintf("%s-%s", resident, now.UTC().Format("20060102T150405.000000000Z")),
		Direction: "resident_to_chenglin",
		Resident:  resident,
		From:      resident,
		To:        "chenglin",
		Body:      strings.TrimSpace(body),
		CreatedAt: now.UTC().Format(time.RFC3339),
	}
	if err := s.append(msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func (s *Store) ReadRecentForResident(resident string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 8
	}

	files, err := filepath.Glob(filepath.Join(s.root, "world", "messages", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	var out []Message
	for i := len(files) - 1; i >= 0; i-- {
		file, err := os.Open(files[i])
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
			if msg.Resident != resident {
				continue
			}
			out = append(out, msg)
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		_ = file.Close()
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) append(msg Message) error {
	dir := filepath.Join(s.root, "world", "messages")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	dayFile := filepath.Join(dir, time.Now().UTC().Format("2006-01-02")+".jsonl")
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
