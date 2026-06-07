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
