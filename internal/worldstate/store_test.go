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
	if _, err := store.AppendResidentToChenglin("jade", "hello jade", now.Add(time.Second)); err != nil {
		t.Fatalf("append jade message: %v", err)
	}
	if _, err := store.ReplyToResidentMessage(amberMsg.ID, "reply one", now.Add(2*time.Second)); err != nil {
		t.Fatalf("reply amber message: %v", err)
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
	if len(jadeThread) != 1 {
		t.Fatalf("expected 1 jade thread message, got %d", len(jadeThread))
	}
	if jadeThread[0].Status != StatusPending {
		t.Fatalf("expected jade request pending, got %s", jadeThread[0].Status)
	}
}

func TestResidentCanMarkReplyRead(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	msg, err := store.AppendResidentToChenglin("amber", "hello", now)
	if err != nil {
		t.Fatalf("append amber: %v", err)
	}
	reply, err := store.ReplyToResidentMessage(msg.ID, "reply one", now.Add(time.Second))
	if err != nil {
		t.Fatalf("reply amber: %v", err)
	}
	if err := store.MarkResidentMessagesRead("amber", []string{reply.ID}, now.Add(2*time.Second)); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	thread, err := store.ReadThreadForResident("amber")
	if err != nil {
		t.Fatalf("amber thread: %v", err)
	}
	if thread[1].Status != StatusDelivered {
		t.Fatalf("expected resident reply delivered, got %s", thread[1].Status)
	}
	if thread[1].ReadAt == "" {
		t.Fatalf("expected resident reply internal read_at to be set")
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
		t.Fatalf("expected chat ignore to fail")
	}
}

func TestTicketLifecycle(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	ticket, err := store.CreateResidentTicket("amber", "Need disk", "Please increase disk to 20G", TicketPriorityHigh, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if ticket.Status != TicketStatusOpen {
		t.Fatalf("expected open ticket, got %s", ticket.Status)
	}

	replied, err := store.ReplyTicket(ticket.ID, "Noted. Evaluating.", false, now.Add(time.Second))
	if err != nil {
		t.Fatalf("reply ticket: %v", err)
	}
	if replied.Status != TicketStatusAnswered {
		t.Fatalf("expected answered ticket, got %s", replied.Status)
	}

	list, err := store.ReadTickets("amber", TicketStatusAnswered, "", 10)
	if err != nil {
		t.Fatalf("read tickets: %v", err)
	}
	if len(list) != 1 || list[0].ID != ticket.ID {
		t.Fatalf("expected answered amber ticket")
	}
}

func TestConsumeFreshTicketUpdatesMarksLatestHostReplySeen(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	ticket, err := store.CreateResidentTicket("amber", "Need disk", "Please increase disk to 20G", TicketPriorityHigh, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := store.ReplyTicket(ticket.ID, "Noted. Evaluating.", false, now.Add(time.Second)); err != nil {
		t.Fatalf("reply ticket: %v", err)
	}

	summaries, fresh, err := store.ConsumeFreshTicketUpdates("amber", 10)
	if err != nil {
		t.Fatalf("consume fresh updates: %v", err)
	}
	if len(summaries) != 1 || len(fresh) != 1 {
		t.Fatalf("expected 1 summary and 1 fresh update, got %d / %d", len(summaries), len(fresh))
	}
	if fresh[0].ID != ticket.ID {
		t.Fatalf("expected fresh update for ticket %s, got %s", ticket.ID, fresh[0].ID)
	}

	againSummaries, againFresh, err := store.ConsumeFreshTicketUpdates("amber", 10)
	if err != nil {
		t.Fatalf("consume fresh updates again: %v", err)
	}
	if len(againSummaries) != 1 {
		t.Fatalf("expected 1 summary on second read, got %d", len(againSummaries))
	}
	if len(againFresh) != 0 {
		t.Fatalf("expected no fresh updates after consumption, got %d", len(againFresh))
	}

	updated, err := store.ReadTicket(ticket.ID)
	if err != nil {
		t.Fatalf("read updated ticket: %v", err)
	}
	if updated.ResidentSeenAt == "" {
		t.Fatalf("expected resident_seen_at to be set")
	}
}

func TestReadAllThreadSummaries(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	amberMsg, err := store.AppendResidentToChenglin("amber", "hello from amber", now)
	if err != nil {
		t.Fatalf("append amber: %v", err)
	}
	if _, err := store.ReplyToResidentMessage(amberMsg.ID, "reply amber", now.Add(time.Second)); err != nil {
		t.Fatalf("reply amber: %v", err)
	}
	if _, err := store.AppendResidentToChenglin("jade", "pending jade", now.Add(2*time.Second)); err != nil {
		t.Fatalf("append jade: %v", err)
	}

	summaries, err := store.ReadAllThreadSummaries()
	if err != nil {
		t.Fatalf("thread summaries: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	if summaries[0].Resident != "jade" {
		t.Fatalf("expected jade latest thread first, got %s", summaries[0].Resident)
	}
	if !summaries[0].NeedsHostAttention || summaries[0].PendingCount != 1 {
		t.Fatalf("expected jade to need host attention")
	}
}
