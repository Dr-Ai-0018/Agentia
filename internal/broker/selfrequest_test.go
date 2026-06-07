package broker

import (
	"testing"

	"ai-arena/internal/auth"
	"ai-arena/internal/worldstate"
)

func TestSelfServiceRequestMemoryCreatesTicket(t *testing.T) {
	app := New(t.TempDir())
	service := NewSelfService(app)

	result, err := service.RequestMemory(auth.ResidentClaim{ResidentID: "amber"}, ResourceRequestInput{
		Amount:  "4GiB",
		Reason:  "Current work would benefit from a larger working set.",
		Urgency: "high",
	})
	if err != nil {
		t.Fatalf("request memory: %v", err)
	}
	if result.Resource != "memory" || result.Priority != worldstate.TicketPriorityHigh {
		t.Fatalf("unexpected result: %#v", result)
	}

	ticket, err := worldstate.New(app.root).ReadTicket(result.TicketID)
	if err != nil {
		t.Fatalf("read created ticket: %v", err)
	}
	if ticket.Resident != "amber" {
		t.Fatalf("unexpected resident: %s", ticket.Resident)
	}
}

func TestSelfServiceRequestDiskRequiresReason(t *testing.T) {
	app := New(t.TempDir())
	service := NewSelfService(app)

	if _, err := service.RequestDisk(auth.ResidentClaim{ResidentID: "amber"}, ResourceRequestInput{
		Amount: "20GiB",
	}); err == nil {
		t.Fatalf("expected missing reason to fail")
	}
}

func TestSelfServiceRequestGPUTimeCreatesTicket(t *testing.T) {
	app := New(t.TempDir())
	service := NewSelfService(app)

	result, err := service.RequestGPUTime(auth.ResidentClaim{ResidentID: "amber"}, ResourceRequestInput{
		Amount:  "2h on L4",
		Reason:  "I want to test a heavier model workflow.",
		Urgency: "medium",
	})
	if err != nil {
		t.Fatalf("request gpu time: %v", err)
	}
	ticket, err := worldstate.New(app.root).ReadTicket(result.TicketID)
	if err != nil {
		t.Fatalf("read created ticket: %v", err)
	}
	if ticket.Title != "Request GPU time" {
		t.Fatalf("unexpected ticket title: %s", ticket.Title)
	}
}

func TestSelfServiceSubmitResultCreatesWorldMessage(t *testing.T) {
	app := New(t.TempDir())
	service := NewSelfService(app)

	result, err := service.SubmitResult(auth.ResidentClaim{ResidentID: "amber"}, SubmissionInput{
		Title:   "Baseline complete",
		Summary: "I verified the current VM baseline and left a concise note.",
		Details: "Network, resources, and local notes were inspected.",
	})
	if err != nil {
		t.Fatalf("submit result: %v", err)
	}
	thread, err := worldstate.New(app.root).ReadThreadForResident("amber")
	if err != nil {
		t.Fatalf("read thread: %v", err)
	}
	if len(thread) != 1 || thread[0].ID != result.MessageID {
		t.Fatalf("expected submitted result message in world thread, got %#v", thread)
	}
}
