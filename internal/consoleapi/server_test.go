package consoleapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
}
