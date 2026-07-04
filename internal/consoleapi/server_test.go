package consoleapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ai-arena/internal/worldstate"
)

func TestHealthRoute(t *testing.T) {
	server := New(Options{
		Root: t.TempDir(),
		Now:  func() time.Time { return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC) },
	})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if _, ok := body["root"]; ok {
		t.Fatalf("health must not expose root path: %s", rec.Body.String())
	}
}

func TestHandlerRequiresTokenWhenConfigured(t *testing.T) {
	server := New(Options{
		Root:  t.TempDir(),
		Token: "secret",
		Now:   func() time.Time { return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC) },
	})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("X-Arena-Console-Token", "wrong")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for wrong token; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("X-Arena-Console-Token", "secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestTicketsRoutes(t *testing.T) {
	server := New(Options{
		Root: t.TempDir(),
		Now:  func() time.Time { return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC) },
	})
	ticket, err := server.world.CreateResidentTicket("amber", "Need disk", "Please increase disk", worldstate.TicketPriorityHigh, time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tickets?resident=amber&status=open", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var list []worldstate.ResidentTicketSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode tickets: %v", err)
	}
	if len(list) != 1 || list[0].ID != ticket.ID {
		t.Fatalf("unexpected tickets: %#v", list)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/tickets/"+ticket.ID, nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var full worldstate.Ticket
	if err := json.Unmarshal(rec.Body.Bytes(), &full); err != nil {
		t.Fatalf("decode ticket: %v", err)
	}
	if full.ID != ticket.ID || full.Body == "" {
		t.Fatalf("unexpected ticket body: %#v", full)
	}
}

func TestTicketRouteRejectsUnsafeID(t *testing.T) {
	server := New(Options{Root: t.TempDir()})
	req := httptest.NewRequest(http.MethodGet, "/api/tickets/not-a-ticket", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := newRateLimiter(1, 2)
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	if !limiter.allow("client") || !limiter.allow("client") {
		t.Fatal("expected burst requests to pass")
	}
	if limiter.allow("client") {
		t.Fatal("expected third immediate request to be rate limited")
	}
	now = now.Add(time.Second)
	if !limiter.allow("client") {
		t.Fatal("expected request after refill to pass")
	}
}
