package broker

import (
	"testing"
	"time"

	"ai-arena/internal/auth"
)

func TestSelfServiceStatus(t *testing.T) {
	app := New(t.TempDir())
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)
	if _, err := app.RunReset("amber", now); err != nil {
		t.Fatalf("reset amber: %v", err)
	}

	service := NewSelfService(app)
	status, err := service.Status(auth.ResidentClaim{ResidentID: "amber"})
	if err != nil {
		t.Fatalf("self status: %v", err)
	}
	if status.ResidentID != "amber" {
		t.Fatalf("unexpected resident id: %s", status.ResidentID)
	}
}

func TestSelfServiceBindingDeniedOnMismatch(t *testing.T) {
	app := New(t.TempDir())
	service := NewSelfService(app)
	if _, err := service.Binding(auth.ResidentClaim{ResidentID: "amber"}); err != nil {
		t.Fatalf("expected self binding query to pass: %v", err)
	}
	if err := auth.ValidateSelfAccess(auth.ResidentClaim{ResidentID: "amber"}, "jade"); err == nil {
		t.Fatalf("expected mismatched self access to fail")
	}
}
