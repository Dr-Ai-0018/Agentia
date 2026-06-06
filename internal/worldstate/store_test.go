package worldstate

import (
	"testing"
	"time"
)

func TestReplyAndIgnoreLifecycle(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	amberMsg, err := store.AppendResidentToChenglin("amber", "hello", now)
	if err != nil {
		t.Fatalf("append amber message: %v", err)
	}
	jadeMsg, err := store.AppendResidentToChenglin("jade", "hello jade", now.Add(time.Second))
	if err != nil {
		t.Fatalf("append jade message: %v", err)
	}
	if _, err := store.ReplyToResidentMessage(amberMsg.ID, "reply one", now.Add(2*time.Second)); err != nil {
		t.Fatalf("reply amber message: %v", err)
	}
	if _, err := store.IgnoreResidentMessage(jadeMsg.ID, now.Add(3*time.Second)); err != nil {
		t.Fatalf("ignore jade message: %v", err)
	}

	amberThread, err := store.ReadThreadForResident("amber")
	if err != nil {
		t.Fatalf("amber thread: %v", err)
	}
	if len(amberThread) != 2 {
		t.Fatalf("expected 2 amber thread messages, got %d", len(amberThread))
	}
	if amberThread[0].Status != StatusReplied {
		t.Fatalf("expected amber request replied, got %s", amberThread[0].Status)
	}
	if amberThread[1].Status != StatusDelivered || amberThread[1].Direction != DirectionChenglinToResident {
		t.Fatalf("expected amber reply delivered")
	}

	jadeThread, err := store.ReadThreadForResident("jade")
	if err != nil {
		t.Fatalf("jade thread: %v", err)
	}
	if len(jadeThread) != 2 {
		t.Fatalf("expected 2 jade thread messages, got %d", len(jadeThread))
	}
	if jadeThread[0].Status != StatusIgnored {
		t.Fatalf("expected jade request ignored, got %s", jadeThread[0].Status)
	}
	if !jadeThread[1].DefaultFeedback {
		t.Fatalf("expected system feedback flag")
	}
}

func TestPendingInboxAndStatusFilter(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	amberMsg, err := store.AppendResidentToChenglin("amber", "need answer", now)
	if err != nil {
		t.Fatalf("append amber: %v", err)
	}
	if _, err := store.AppendResidentToChenglin("jade", "still pending", now.Add(time.Second)); err != nil {
		t.Fatalf("append jade: %v", err)
	}
	if _, err := store.ReplyToResidentMessage(amberMsg.ID, "handled", now.Add(2*time.Second)); err != nil {
		t.Fatalf("reply amber: %v", err)
	}

	pending, err := store.ReadPendingResidentMessages(10)
	if err != nil {
		t.Fatalf("pending inbox: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending message, got %d", len(pending))
	}
	if pending[0].Resident != "jade" {
		t.Fatalf("expected jade pending, got %s", pending[0].Resident)
	}

	replied, err := store.ReadMessagesByStatus("amber", StatusReplied, 10)
	if err != nil {
		t.Fatalf("replied status: %v", err)
	}
	if len(replied) != 1 || replied[0].ID != amberMsg.ID {
		t.Fatalf("expected original amber request in replied filter")
	}
}

func TestCannotProcessHandledMessageTwice(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	msg, err := store.AppendResidentToChenglin("amber", "once only", now)
	if err != nil {
		t.Fatalf("append amber: %v", err)
	}
	if _, err := store.ReplyToResidentMessage(msg.ID, "handled", now.Add(time.Second)); err != nil {
		t.Fatalf("reply amber: %v", err)
	}
	if _, err := store.IgnoreResidentMessage(msg.ID, now.Add(2*time.Second)); err == nil {
		t.Fatalf("expected ignore after reply to fail")
	}
}
