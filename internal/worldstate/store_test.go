package worldstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReplyLifecycle(t *testing.T) {
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

func TestCannotReplyHandledMessageTwice(t *testing.T) {
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
	if _, err := store.ReplyToResidentMessage(msg.ID, "handled again", now.Add(2*time.Second)); err == nil {
		t.Fatalf("expected second chat reply to fail")
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

func TestConsumeFreshHostInterventionsMarksSeen(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	item, err := store.CreateHostIntervention("amber", "maintenance", "Planned maintenance", "A short maintenance window is scheduled.", "chenglin", now)
	if err != nil {
		t.Fatalf("create host intervention: %v", err)
	}

	summaries, fresh, err := store.ConsumeFreshHostInterventions("amber", 10)
	if err != nil {
		t.Fatalf("consume fresh interventions: %v", err)
	}
	if len(summaries) != 1 || len(fresh) != 1 {
		t.Fatalf("expected 1 summary and 1 fresh intervention, got %d / %d", len(summaries), len(fresh))
	}
	if fresh[0].ID != item.ID {
		t.Fatalf("expected fresh intervention %s, got %s", item.ID, fresh[0].ID)
	}

	_, againFresh, err := store.ConsumeFreshHostInterventions("amber", 10)
	if err != nil {
		t.Fatalf("consume fresh interventions again: %v", err)
	}
	if len(againFresh) != 0 {
		t.Fatalf("expected no fresh interventions after consumption, got %d", len(againFresh))
	}
}

func TestUpdateLatestOpenHostInterventionChangesStatus(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	created, err := store.CreateHostIntervention("amber", "maintenance", "Planned maintenance", "Scheduled maintenance is approved.", "chenglin", now)
	if err != nil {
		t.Fatalf("create host intervention: %v", err)
	}
	if created.Status != "planned" {
		t.Fatalf("expected planned intervention, got %#v", created)
	}

	updated, ok, err := store.UpdateLatestOpenHostIntervention("amber", "maintenance", "Planned maintenance", "in_progress", "Maintenance has started.\nmaintenance_started=true\nmaintenance_state=in_progress\noperator=chenglin", "chenglin", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("update intervention status: %v", err)
	}
	if !ok {
		t.Fatalf("expected matching intervention to update")
	}
	if updated.Status != "in_progress" {
		t.Fatalf("expected in_progress intervention, got %#v", updated)
	}
	if !strings.Contains(updated.Body, "Maintenance has started.") {
		t.Fatalf("expected updated body, got %#v", updated)
	}
	items, err := store.ReadHostInterventions("amber", "", 10)
	if err != nil {
		t.Fatalf("read interventions: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one intervention summary, got %#v", items)
	}
	if items[0].Maintenance["maintenance_state"] != "in_progress" || items[0].Maintenance["operator"] != "chenglin" {
		t.Fatalf("expected structured maintenance metadata, got %#v", items[0].Maintenance)
	}

	followups, err := store.ReadHostFollowups(10)
	if err != nil {
		t.Fatalf("read host followups: %v", err)
	}
	if len(followups) != 1 {
		t.Fatalf("expected one followup, got %#v", followups)
	}
	if followups[0].Maintenance["maintenance_state"] != "in_progress" {
		t.Fatalf("expected followup maintenance metadata, got %#v", followups[0].Maintenance)
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

func TestReadHostInboxSummary(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	if _, err := store.AppendResidentToChenglin("jade", "pending jade", now); err != nil {
		t.Fatalf("append jade: %v", err)
	}
	if _, err := store.AppendResidentToChenglin("jade", "pending jade again", now.Add(time.Second)); err != nil {
		t.Fatalf("append jade second: %v", err)
	}
	if _, err := store.CreateResidentTicket("amber", "Need disk", "Please increase disk to 20G", TicketPriorityHigh, now.Add(time.Second)); err != nil {
		t.Fatalf("create amber ticket: %v", err)
	}

	summary, err := store.ReadHostInboxSummary(10, 10)
	if err != nil {
		t.Fatalf("host inbox summary: %v", err)
	}
	if summary.ResidentsNeedingChatReply != 1 {
		t.Fatalf("expected 1 resident needing chat reply, got %d", summary.ResidentsNeedingChatReply)
	}
	if summary.ResidentsWithOpenTickets != 1 {
		t.Fatalf("expected 1 resident with open tickets, got %d", summary.ResidentsWithOpenTickets)
	}
	if len(summary.PendingChatMessages) != 1 || summary.PendingChatMessages[0].Resident != "jade" {
		t.Fatalf("expected pending jade chat, got %#v", summary.PendingChatMessages)
	}
	if summary.PendingChatMessages[0].Body != "pending jade again" {
		t.Fatalf("expected latest pending jade chat, got %#v", summary.PendingChatMessages[0])
	}
	if len(summary.OpenTickets) != 1 || summary.OpenTickets[0].Resident != "amber" {
		t.Fatalf("expected amber open ticket, got %#v", summary.OpenTickets)
	}
	if len(summary.ThreadSummaries) != 1 || summary.ThreadSummaries[0].Resident != "jade" {
		t.Fatalf("expected jade thread summary, got %#v", summary.ThreadSummaries)
	}
}

func TestReadHostFollowups(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	if _, err := store.AppendResidentToChenglin("jade", "pending jade", now); err != nil {
		t.Fatalf("append jade: %v", err)
	}
	if _, err := store.AppendResidentToChenglin("jade", "pending jade again", now.Add(time.Second)); err != nil {
		t.Fatalf("append jade second: %v", err)
	}
	ticket, err := store.CreateResidentTicket("amber", "Need disk", "Please increase disk to 20G", TicketPriorityHigh, now.Add(time.Second))
	if err != nil {
		t.Fatalf("create amber ticket: %v", err)
	}

	items, err := store.ReadHostFollowups(10)
	if err != nil {
		t.Fatalf("host followups: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 followups, got %d", len(items))
	}
	if items[0].Kind != "chat_reply" || items[0].Resident != "jade" {
		t.Fatalf("expected latest jade chat followup first, got %#v", items[0])
	}
	if items[1].Kind != "ticket_reply" || items[1].TargetID != ticket.ID {
		t.Fatalf("expected ticket followup second, got %#v", items[1])
	}
	if items[0].Preview != previewText("pending jade again", 160) {
		t.Fatalf("expected latest jade followup preview, got %#v", items[0])
	}
}

func TestMarkResidentMessagesReadDoesNotLeaveTempFiles(t *testing.T) {
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

	matches, err := filepath.Glob(filepath.Join(root, "world", "messages", "*.tmp"))
	if err != nil {
		t.Fatalf("glob tmp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no temp files, got %#v", matches)
	}
}

func TestWriteTicketDoesNotLeaveTempFiles(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	ticket, err := store.CreateResidentTicket("amber", "Need disk", "Please increase disk", TicketPriorityHigh, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "world", "tickets", ticket.ID+".json")); err != nil {
		t.Fatalf("expected ticket file: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, "world", "tickets", "*.tmp"))
	if err != nil {
		t.Fatalf("glob tmp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no temp files, got %#v", matches)
	}
}

func TestMarkResidentMessagesReadDoesNotModifyTickets(t *testing.T) {
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
	ticket, err := store.CreateResidentTicket("amber", "Need disk", "Please increase disk", TicketPriorityHigh, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	ticketPath := filepath.Join(root, "world", "tickets", ticket.ID+".json")
	before, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("read ticket before mark-read: %v", err)
	}

	if err := store.MarkResidentMessagesRead("amber", []string{reply.ID}, now.Add(3*time.Second)); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	after, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("read ticket after mark-read: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected ticket file to remain unchanged during message read marking")
	}
}

func TestRewriteAllCheckedDetectsMessageFileConflict(t *testing.T) {
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

	all, fileState, err := store.readAllWithState()
	if err != nil {
		t.Fatalf("read all with state: %v", err)
	}
	for i := range all {
		if all[i].ID == reply.ID {
			all[i].ReadAt = now.Add(2 * time.Second).UTC().Format(time.RFC3339)
		}
	}

	if _, err := store.AppendResidentToChenglin("amber", "late new message", now.Add(3*time.Second)); err != nil {
		t.Fatalf("append late amber message: %v", err)
	}

	err = store.rewriteAllChecked(all, fileState)
	if !errors.Is(err, ErrMessageFileConflict) {
		t.Fatalf("expected ErrMessageFileConflict, got %v", err)
	}

	thread, err := store.ReadThreadForResident("amber")
	if err != nil {
		t.Fatalf("read amber thread: %v", err)
	}
	if len(thread) != 3 {
		t.Fatalf("expected appended message to survive conflict path, got %d thread items", len(thread))
	}
}

func TestQuarantineMessageFile(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	if _, err := store.AppendResidentToChenglin("amber", "hello", now); err != nil {
		t.Fatalf("append amber: %v", err)
	}
	path := filepath.Join(root, "world", "messages", "2026-06-06.jsonl")
	dst, err := store.QuarantineMessageFile(path, now.Add(time.Second), "corrupt")
	if err != nil {
		t.Fatalf("quarantine message file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected original message file to be moved away, stat err=%v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("expected quarantined message file to exist: %v", err)
	}
	if !strings.HasSuffix(dst, ".corrupt.bak") {
		t.Fatalf("expected quarantined file suffix, got %s", dst)
	}
}

func TestMessagesAndTicketsStayConsistentAcrossMixedOperations(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	amberMsg, err := store.AppendResidentToChenglin("amber", "hello", now)
	if err != nil {
		t.Fatalf("append amber message: %v", err)
	}
	if _, err := store.ReplyToResidentMessage(amberMsg.ID, "reply one", now.Add(time.Second)); err != nil {
		t.Fatalf("reply amber message: %v", err)
	}
	ticket, err := store.CreateResidentTicket("amber", "Need disk", "Please increase disk", TicketPriorityHigh, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := store.ReplyTicket(ticket.ID, "Approved later", false, now.Add(3*time.Second)); err != nil {
		t.Fatalf("reply ticket: %v", err)
	}

	thread, err := store.ReadThreadForResident("amber")
	if err != nil {
		t.Fatalf("read amber thread: %v", err)
	}
	if len(thread) != 2 {
		t.Fatalf("expected 2 thread messages, got %d", len(thread))
	}
	if thread[0].Status != StatusReplied || thread[1].Status != StatusDelivered {
		t.Fatalf("unexpected thread state after mixed operations: %#v", thread)
	}

	tickets, err := store.ReadTickets("amber", "", "", 10)
	if err != nil {
		t.Fatalf("read tickets: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("expected 1 ticket summary, got %d", len(tickets))
	}
	if tickets[0].Status != TicketStatusAnswered {
		t.Fatalf("expected answered ticket after host reply, got %#v", tickets[0])
	}

	inbox, err := store.ReadHostInboxSummary(10, 10)
	if err != nil {
		t.Fatalf("read host inbox: %v", err)
	}
	if inbox.ResidentsNeedingChatReply != 0 {
		t.Fatalf("expected no pending host chat replies, got %#v", inbox)
	}
	if inbox.ResidentsWithOpenTickets != 0 {
		t.Fatalf("expected no open tickets after ticket reply, got %#v", inbox)
	}
}
