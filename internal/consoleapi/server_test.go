package consoleapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/orchestrator"
	"ai-arena/internal/runtime/newborn"
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

func TestPreflightRouteReturnsChecklist(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	server := New(Options{
		Root:  t.TempDir(),
		Token: "secret",
		Now:   func() time.Time { return now },
	})
	req := httptest.NewRequest(http.MethodGet, "/api/preflight", nil)
	req.Header.Set("X-Arena-Console-Token", "secret")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out PreflightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode preflight: %v", err)
	}
	if out.GeneratedAt != now.Format(time.RFC3339) {
		t.Fatalf("generated_at = %q", out.GeneratedAt)
	}
	if out.Overall != "good" {
		t.Fatalf("overall = %q, want good; checks=%#v", out.Overall, out.Checks)
	}
	checks := preflightChecksByID(out.Checks)
	for _, id := range []string{"service_running", "backend_token", "public_basic_auth", "runs_readable", "budget_readable", "inbox_readable", "followups_readable", "tickets_readable", "recent_5xx", "rate_limit", "soak_gate"} {
		if _, ok := checks[id]; !ok {
			t.Fatalf("missing check %q in %#v", id, out.Checks)
		}
	}
	if checks["backend_token"].Status != "good" || !checks["backend_token"].Required {
		t.Fatalf("backend token check = %#v", checks["backend_token"])
	}
	if checks["public_basic_auth"].Status != "unknown" || checks["public_basic_auth"].Required {
		t.Fatalf("public basic auth check = %#v", checks["public_basic_auth"])
	}
}

func TestPreflightWatchesMissingBackendToken(t *testing.T) {
	server := New(Options{
		Root: t.TempDir(),
		Now:  func() time.Time { return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC) },
	})
	req := httptest.NewRequest(http.MethodGet, "/api/preflight", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out PreflightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode preflight: %v", err)
	}
	checks := preflightChecksByID(out.Checks)
	if got := checks["backend_token"].Status; got != "watch" {
		t.Fatalf("backend token status = %q, want watch", got)
	}
	if out.Overall != "watch" {
		t.Fatalf("overall = %q, want watch", out.Overall)
	}
}

func TestPreflightReportsAccessWarnings(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	server := New(Options{
		Root:  t.TempDir(),
		Token: "secret",
		Now:   func() time.Time { return now },
	})
	server.access.record(http.StatusInternalServerError, now.Add(-time.Minute))
	server.access.record(http.StatusTooManyRequests, now.Add(-30*time.Second))
	req := httptest.NewRequest(http.MethodGet, "/api/preflight", nil)
	req.Header.Set("X-Arena-Console-Token", "secret")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out PreflightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode preflight: %v", err)
	}
	checks := preflightChecksByID(out.Checks)
	if got := checks["recent_5xx"].Status; got != "watch" {
		t.Fatalf("recent_5xx status = %q, want watch", got)
	}
	if got := checks["rate_limit"].Status; got != "watch" {
		t.Fatalf("rate_limit status = %q, want watch", got)
	}
	if out.Overall != "watch" {
		t.Fatalf("overall = %q, want watch", out.Overall)
	}
}

func TestInspectSummaryRouteUsesCachedSnapshot(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	server := New(Options{
		Root: root,
		Now:  func() time.Time { return now },
	})
	writeConsoleInventorySnapshot(t, root, now)

	req := httptest.NewRequest(http.MethodGet, "/api/system/inspect-summary", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out broker.HostInspectSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode inspect summary: %v", err)
	}
	if out.ResidentCount != 3 {
		t.Fatalf("resident count = %d, want 3", out.ResidentCount)
	}
}

func TestAcceptanceRouteUsesCachedSnapshot(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	server := New(Options{
		Root: root,
		Now:  func() time.Time { return now },
	})
	writeConsoleInventorySnapshot(t, root, now)

	req := httptest.NewRequest(http.MethodGet, "/api/acceptance", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out broker.V0AcceptanceOutput
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode acceptance: %v", err)
	}
	if out.Source != "cached" {
		t.Fatalf("source = %q, want cached", out.Source)
	}
}

func TestSummaryDoesNotExposeStaleRunAsActive(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	server := New(Options{
		Root: root,
		Now:  func() time.Time { return now },
	})
	server.orchestrator = orchestrator.New(server.broker, &http.Client{}, "", "")
	server.orchestrator.SetStateRootForTest(filepath.Join(root, "orchestrator-runs"))

	runID := "orchestrator-20260713T052805.776535403Z"
	runDir := filepath.Join(root, "orchestrator-runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	status := orchestrator.RunStatus{
		RunID:     runID,
		Status:    "running",
		Mode:      orchestrator.RunModeParallel,
		StartedAt: "2026-07-13T05:28:05Z",
		UpdatedAt: "2026-07-13T05:33:05Z",
		Residents: []orchestrator.ResidentRunStatus{{
			Resident:  "jade",
			Status:    "running",
			UpdatedAt: "2026-07-13T05:33:05Z",
		}},
	}
	writeTestJSON(t, filepath.Join(runDir, "run-status.json"), status)

	req := httptest.NewRequest(http.MethodGet, "/api/summary", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out DashboardSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if out.ActiveRun != nil {
		t.Fatalf("stale run must not be active: %#v", out.ActiveRun)
	}
	if out.LatestRun == nil || out.LatestRun.RunID != runID {
		t.Fatalf("latest run evidence should remain visible, got %#v", out.LatestRun)
	}
	if out.LatestRun.Status != "abandoned" {
		t.Fatalf("stale latest run status = %q, want abandoned", out.LatestRun.Status)
	}
	foundStaleAlert := false
	for _, alert := range out.Alerts {
		if alert.Kind == "run_stale" && alert.RunID == runID {
			foundStaleAlert = true
		}
	}
	if !foundStaleAlert {
		t.Fatalf("expected stale run alert in %#v", out.Alerts)
	}
}

func TestSummaryKeepsFreshRunActive(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	server := New(Options{
		Root: root,
		Now:  func() time.Time { return now },
	})
	server.orchestrator = orchestrator.New(server.broker, &http.Client{}, "", "")
	server.orchestrator.SetStateRootForTest(filepath.Join(root, "orchestrator-runs"))

	runID := "orchestrator-20260726T005500.000000000Z"
	runDir := filepath.Join(root, "orchestrator-runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	status := orchestrator.RunStatus{
		RunID:     runID,
		Status:    "running",
		Mode:      orchestrator.RunModeParallel,
		StartedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
		UpdatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
		Residents: []orchestrator.ResidentRunStatus{{
			Resident:  "jade",
			Status:    "running",
			UpdatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
		}},
	}
	writeTestJSON(t, filepath.Join(runDir, "run-status.json"), status)

	req := httptest.NewRequest(http.MethodGet, "/api/summary", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out DashboardSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if out.ActiveRun == nil || out.ActiveRun.RunID != runID {
		t.Fatalf("fresh run should remain active, got %#v", out.ActiveRun)
	}
}

func TestRunsMarksStalePersistedRunningAsAbandoned(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	server := New(Options{
		Root: root,
		Now:  func() time.Time { return now },
	})
	server.orchestrator = orchestrator.New(server.broker, &http.Client{}, "", "")
	server.orchestrator.SetStateRootForTest(filepath.Join(root, "orchestrator-runs"))

	staleID := "orchestrator-20260713T052805.776535403Z"
	staleDir := filepath.Join(root, "orchestrator-runs", staleID)
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("mkdir stale run dir: %v", err)
	}
	writeTestJSON(t, filepath.Join(staleDir, "run-status.json"), orchestrator.RunStatus{
		RunID:     staleID,
		Status:    "running",
		Mode:      orchestrator.RunModeParallel,
		StartedAt: "2026-07-13T05:28:05Z",
		UpdatedAt: "2026-07-13T05:33:05Z",
		Residents: []orchestrator.ResidentRunStatus{{
			Resident:  "jade",
			Status:    "running",
			UpdatedAt: "2026-07-13T05:33:05Z",
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out []orchestrator.RunRecord
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("runs len = %d, want 1: %#v", len(out), out)
	}
	if out[0].Status != "abandoned" {
		t.Fatalf("stale persisted running status = %q, want abandoned", out[0].Status)
	}
}

func TestSummarySuppressesHistoricalStaleAlertsAfterNewerFinishedRun(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	server := New(Options{
		Root: root,
		Now:  func() time.Time { return now },
	})
	server.orchestrator = orchestrator.New(server.broker, &http.Client{}, "", "")
	server.orchestrator.SetStateRootForTest(filepath.Join(root, "orchestrator-runs"))

	finishedID := "orchestrator-20260726T000000.000000000Z"
	finishedDir := filepath.Join(root, "orchestrator-runs", finishedID)
	if err := os.MkdirAll(finishedDir, 0o755); err != nil {
		t.Fatalf("mkdir finished run dir: %v", err)
	}
	writeTestJSON(t, filepath.Join(finishedDir, "run-status.json"), orchestrator.RunStatus{
		RunID:      finishedID,
		Status:     "finished",
		Mode:       orchestrator.RunModeParallel,
		StartedAt:  now.Add(-2 * time.Hour).Format(time.RFC3339),
		UpdatedAt:  now.Add(-1 * time.Hour).Format(time.RFC3339),
		FinishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
	})

	staleID := "orchestrator-20260713T052805.776535403Z"
	staleDir := filepath.Join(root, "orchestrator-runs", staleID)
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("mkdir stale run dir: %v", err)
	}
	writeTestJSON(t, filepath.Join(staleDir, "run-status.json"), orchestrator.RunStatus{
		RunID:     staleID,
		Status:    "running",
		Mode:      orchestrator.RunModeParallel,
		StartedAt: "2026-07-13T05:28:05Z",
		UpdatedAt: "2026-07-13T05:33:05Z",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/summary", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out DashboardSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if out.ActiveRun != nil {
		t.Fatalf("no run should be active, got %#v", out.ActiveRun)
	}
	for _, alert := range out.Alerts {
		if alert.Kind == "run_stale" && alert.RunID == staleID {
			t.Fatalf("older abandoned run should not pollute current alerts: %#v", out.Alerts)
		}
	}
	if len(out.Runs) < 2 || out.Runs[1].Status != "abandoned" {
		t.Fatalf("older stale run should stay visible as abandoned evidence: %#v", out.Runs)
	}
}

func TestCompactionDiagnosticsRouteAggregatesRunReports(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	server := New(Options{
		Root: root,
		Now:  func() time.Time { return now },
	})
	server.orchestrator = orchestrator.New(server.broker, &http.Client{}, "", "")
	server.orchestrator.SetStateRootForTest(filepath.Join(root, "orchestrator-runs"))
	runID := "orchestrator-20260713T100000.000000000Z"
	runDir := filepath.Join(root, "orchestrator-runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	status := orchestrator.RunStatus{
		RunID:     runID,
		Status:    "finished",
		Mode:      orchestrator.RunModeParallel,
		StartedAt: "2026-07-13T10:00:00Z",
		UpdatedAt: "2026-07-13T10:15:00Z",
		Residents: []orchestrator.ResidentRunStatus{{
			Resident:  "jade",
			Status:    "finished",
			UpdatedAt: "2026-07-13T10:15:00Z",
		}},
	}
	summary := orchestrator.RunSummary{
		RunID:     runID,
		StartedAt: "2026-07-13T10:00:00Z",
		Mode:      orchestrator.RunModeParallel,
		Contract: orchestrator.RunContract{
			RunID:   runID,
			Mode:    orchestrator.RunModeParallel,
			Purpose: "long-context-smoke",
		},
		Runs: []orchestrator.ResidentRun{{
			Resident: "jade",
			Status:   "ok",
			Report: &newborn.FinalReport{
				Resident:  "jade",
				StartedAt: "2026-07-13T10:00:01Z",
				SummaryPane: &newborn.SummaryPane{
					ApproxTokens: 42,
				},
				CompactionEvents: []newborn.CompactionEvent{{
					CompactionID:                   "compact-jade-1",
					RunID:                          runID,
					Resident:                       "jade",
					OccurredAt:                     "2026-07-13T10:04:00Z",
					TriggerReason:                  newborn.CompactionTriggerPreflightMeasured,
					TriggerDetail:                  "measured prompt",
					TokensBefore:                   200000,
					TokensAfter:                    90000,
					RoundsAbsorbed:                 12,
					SummaryPaneTokensAfter:         42,
					Outcome:                        newborn.CompactionOutcomeSummarized,
					DurationMs:                     1200,
					CachePrefixHitOnCompactionCall: true,
					ProviderUsage: &newborn.BrokerUsageLog{
						ProviderCostRecorded: true,
						CostClass:            "continuity_system",
						PreparedSparkCost:    1.25,
					},
				}},
			},
		}},
	}
	writeTestJSON(t, filepath.Join(runDir, "run-status.json"), status)
	writeTestJSON(t, filepath.Join(runDir, "summary.json"), summary)

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics/compaction", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out CompactionDiagnosticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	if out.GeneratedAt != now.Format(time.RFC3339) {
		t.Fatalf("generatedAt = %q", out.GeneratedAt)
	}
	if len(out.Runs) != 1 || out.Runs[0].RunID != runID || out.Runs[0].Resident != "jade" {
		t.Fatalf("unexpected run diagnostics: %#v", out.Runs)
	}
	if out.Runs[0].TriggerBreakdown["preflight_measured"] != 1 || out.Runs[0].OutcomeBreakdown["summarized"] != 1 {
		t.Fatalf("unexpected breakdown: %#v %#v", out.Runs[0].TriggerBreakdown, out.Runs[0].OutcomeBreakdown)
	}
	if out.Runs[0].CacheHitRateOnCompactionCall != 1 || out.Runs[0].LatestSummaryPaneTokens != 42 {
		t.Fatalf("unexpected run metrics: %#v", out.Runs[0])
	}
	if out.Runs[0].ProviderCostOnlySpark != 1.25 || out.Runs[0].ProviderCostOnlyUSD != 0.0125 || out.Runs[0].ProviderUsageMissing != 0 {
		t.Fatalf("unexpected provider-only cost metrics: %#v", out.Runs[0])
	}
	if len(out.RecentEvents) != 1 || out.RecentEvents[0].CompactionID != "compact-jade-1" || out.RecentEvents[0].TokensAfter != 90000 {
		t.Fatalf("unexpected events: %#v", out.RecentEvents)
	}
	if !out.RecentEvents[0].ProviderUsageRecorded || out.RecentEvents[0].ProviderCostOnlySpark != 1.25 || out.RecentEvents[0].ProviderCostClass != "continuity_system" {
		t.Fatalf("unexpected event provider cost fields: %#v", out.RecentEvents[0])
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal test json: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeConsoleInventorySnapshot(t *testing.T, root string, now time.Time) {
	t.Helper()
	if _, err := broker.SaveInventorySnapshot(root, broker.InventorySnapshot{
		CollectedAt: now.Format(time.RFC3339),
		Residents: []broker.ResidentInventoryFact{
			{
				ResidentID:     "amber",
				InstanceName:   "amber",
				Status:         "Running",
				Type:           "virtual-machine",
				VCPU:           2,
				MemoryLimitMiB: 2048,
				DiskGiB:        20,
				IPv4:           "10.0.0.2",
				UpdatedAt:      now.Format(time.RFC3339),
			},
			{
				ResidentID:     "jade",
				InstanceName:   "jade",
				Status:         "Running",
				Type:           "virtual-machine",
				VCPU:           2,
				MemoryLimitMiB: 2048,
				DiskGiB:        20,
				IPv4:           "10.0.0.3",
				UpdatedAt:      now.Format(time.RFC3339),
			},
			{
				ResidentID:     "onyx",
				InstanceName:   "onyx",
				Status:         "Running",
				Type:           "virtual-machine",
				VCPU:           2,
				MemoryLimitMiB: 2048,
				DiskGiB:        20,
				IPv4:           "10.0.0.4",
				UpdatedAt:      now.Format(time.RFC3339),
			},
		},
	}); err != nil {
		t.Fatalf("save inventory snapshot: %v", err)
	}
}

func preflightChecksByID(checks []PreflightCheck) map[string]PreflightCheck {
	out := make(map[string]PreflightCheck, len(checks))
	for _, check := range checks {
		out[check.ID] = check
	}
	return out
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
