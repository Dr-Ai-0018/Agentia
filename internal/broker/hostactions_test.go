package broker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-arena/internal/worldstate"
)

func TestHostActionServiceReplyAndIgnoreWriteAudit(t *testing.T) {
	root := t.TempDir()
	store := worldstate.New(root)
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	msg, err := store.AppendResidentToChenglin("amber", "hello", now)
	if err != nil {
		t.Fatalf("append resident message: %v", err)
	}

	service := NewHostActionService(root)
	if _, err := service.Reply(msg.ID, "reply"); err != nil {
		t.Fatalf("host reply: %v", err)
	}

	msg2, err := store.AppendResidentToChenglin("amber", "another", now.Add(time.Second))
	if err != nil {
		t.Fatalf("append second message: %v", err)
	}
	if _, err := service.Ignore(msg2.ID); err != nil {
		t.Fatalf("host ignore: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(root, "audit", "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected audit file, got files=%#v err=%v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected audit content")
	}
}

func TestHostActionServiceReplyTicketWritesHistory(t *testing.T) {
	root := t.TempDir()
	store := worldstate.New(root)
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	ticket, err := store.CreateResidentTicket("amber", "Need disk", "Please increase disk", worldstate.TicketPriorityHigh, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	service := NewHostActionService(root)
	if _, err := service.ReplyTicket(ticket.ID, "No change, test only.", true); err != nil {
		t.Fatalf("reply ticket: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(root, "world", "public-history-*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected public history file, got files=%#v err=%v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read public history file: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected public history content")
	}
}
