package consoleapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/orchestrator"
	"ai-arena/internal/worldstate"
)

type Server struct {
	root         string
	token        string
	broker       *broker.App
	world        *worldstate.Store
	actions      *broker.HostActionService
	orchestrator *orchestrator.Service
	now          func() time.Time
	limiter      *rateLimiter
	startedAt    time.Time
	access       *accessStats
}

type Options struct {
	Root  string
	Token string
	Now   func() time.Time
}

func New(options Options) *Server {
	root := strings.TrimSpace(options.Root)
	if root == "" {
		root = ".agents"
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	app := broker.New(root)
	return &Server{
		root:    root,
		token:   strings.TrimSpace(options.Token),
		broker:  app,
		world:   worldstate.New(root),
		actions: broker.NewHostActionService(root),
		// Empty endpoint/auth args select the local filesystem-backed run registry.
		orchestrator: orchestrator.New(app, &http.Client{Timeout: 30 * time.Second}, "", ""),
		now:          now,
		limiter:      newRateLimiter(10, 20),
		startedAt:    now(),
		access:       &accessStats{},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/preflight", s.handlePreflight)
	mux.HandleFunc("GET /api/summary", s.handleSummary)
	mux.HandleFunc("GET /api/runs", s.handleRuns)
	mux.HandleFunc("GET /api/runs/{runID}/status", s.handleRunStatus)
	mux.HandleFunc("GET /api/runs/{runID}/summary", s.handleRunSummary)
	mux.HandleFunc("GET /api/runs/{runID}/report", s.handleRunReport)
	mux.HandleFunc("GET /api/diagnostics/compaction", s.handleCompactionDiagnostics)
	mux.HandleFunc("GET /api/budget", s.handleBudget)
	mux.HandleFunc("GET /api/inbox", s.handleInbox)
	mux.HandleFunc("GET /api/followups", s.handleFollowups)
	mux.HandleFunc("GET /api/tickets", s.handleTickets)
	mux.HandleFunc("GET /api/tickets/{ticketID}", s.handleTicket)
	mux.HandleFunc("GET /api/messages/{resident}", s.handleMessages)
	mux.HandleFunc("GET /api/messages/{resident}/thread", s.handleThread)
	mux.HandleFunc("GET /api/system/inspect-summary", s.handleInspectSummary)
	mux.HandleFunc("GET /api/acceptance", s.handleAcceptance)
	mux.HandleFunc("GET /api/acceptance/evidence", s.handleAcceptanceEvidence)
	mux.HandleFunc("POST /api/reply", s.handleReply)
	mux.HandleFunc("POST /api/ticket-reply", s.handleTicketReply)
	return s.withAccessLog(withJSONHeaders(s.withRateLimit(s.withAuth(mux))))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.access.record(rec.status, s.now())
			log.Printf("consoleapi method=%s path=%s status=%d latency=%s remote=%s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond), r.RemoteAddr)
		}
	})
}

func (s *Server) withRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && !s.limiter.allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", errors.New("rate limited"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	if s.token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && !s.authorized(r.Header.Get("X-Arena-Console-Token")) {
			writeError(w, http.StatusUnauthorized, "unauthorized", errors.New("unauthorized"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" || len(token) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func withJSONHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"generated_at": s.now().Format(time.RFC3339),
	})
}

func (s *Server) handlePreflight(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	checks := []PreflightCheck{
		{
			ID:       "service_running",
			Section:  "service",
			Label:    "console 服务",
			Status:   "good",
			Required: true,
			Detail:   "console API 正在运行。",
			Data: map[string]any{
				"started_at": s.startedAt.Format(time.RFC3339),
				"uptime_sec": int(now.Sub(s.startedAt).Seconds()),
			},
		},
	}

	if s.token == "" {
		checks = append(checks, PreflightCheck{
			ID:       "backend_token",
			Section:  "auth",
			Label:    "后端通行口令",
			Status:   "watch",
			Required: true,
			Detail:   "ARENA_CONSOLE_TOKEN 未启用，/api 缺少后端口令保护。",
		})
	} else {
		checks = append(checks, PreflightCheck{
			ID:       "backend_token",
			Section:  "auth",
			Label:    "后端通行口令",
			Status:   "good",
			Required: true,
			Detail:   "后端 /api 需要 nginx 注入的通行口令。",
		})
	}
	checks = append(checks, PreflightCheck{
		ID:       "public_basic_auth",
		Section:  "auth",
		Label:    "公网入口",
		Status:   "unknown",
		Required: false,
		Detail:   "basic auth 在 nginx 层，后端只能通过外部 smoke 验证。",
	})

	checks = append(checks,
		s.readinessCheck("runs_readable", "storage", "运行记录", func() (map[string]any, error) {
			runs, err := s.orchestrator.ListRuns(1)
			return map[string]any{"sample_count": len(runs)}, err
		}),
		s.readinessCheck("budget_readable", "storage", "额度账本", func() (map[string]any, error) {
			_, err := s.broker.RunBudgetStatus(nil)
			return nil, err
		}),
		s.readinessCheck("inbox_readable", "storage", "程林回话队列", func() (map[string]any, error) {
			_, err := s.world.ReadHostInboxSummary(1, 1)
			return nil, err
		}),
		s.readinessCheck("followups_readable", "storage", "待回应线索", func() (map[string]any, error) {
			followups, err := s.world.ReadHostFollowups(1)
			return map[string]any{"sample_count": len(followups)}, err
		}),
		s.readinessCheck("tickets_readable", "storage", "住户请求", func() (map[string]any, error) {
			tickets, err := s.world.ReadTickets("", "", "", 1)
			return map[string]any{"sample_count": len(tickets)}, err
		}),
	)

	stats := s.access.snapshot()
	if stats.total5xx == 0 {
		checks = append(checks, PreflightCheck{
			ID:       "recent_5xx",
			Section:  "health",
			Label:    "接口错误",
			Status:   "good",
			Required: true,
			Detail:   "本服务进程启动后没有记录到 5xx。",
			Data:     map[string]any{"total_5xx": 0},
		})
	} else {
		checks = append(checks, PreflightCheck{
			ID:       "recent_5xx",
			Section:  "health",
			Label:    "接口错误",
			Status:   "watch",
			Required: true,
			Detail:   "本服务进程启动后记录到 5xx，长测前需要查看 journal。",
			Data: map[string]any{
				"total_5xx":   stats.total5xx,
				"last_5xx_at": stats.last5xx.Format(time.RFC3339),
				"total_seen":  stats.total,
				"started_at":  s.startedAt.Format(time.RFC3339),
			},
		})
	}
	if stats.rateLimited == 0 {
		checks = append(checks, PreflightCheck{
			ID:       "rate_limit",
			Section:  "health",
			Label:    "请求节奏",
			Status:   "good",
			Required: false,
			Detail:   "本服务进程启动后没有触发 rate limit。",
			Data:     map[string]any{"rate_limited": 0},
		})
	} else {
		checks = append(checks, PreflightCheck{
			ID:       "rate_limit",
			Section:  "health",
			Label:    "请求节奏",
			Status:   "watch",
			Required: false,
			Detail:   "本服务进程启动后触发过 rate limit，可能是轮询或外部访问太密。",
			Data: map[string]any{
				"rate_limited":         stats.rateLimited,
				"last_rate_limited_at": stats.lastRateLimited.Format(time.RFC3339),
			},
		})
	}

	overall := preflightOverall(checks)
	summary := "长测前核心检查通过。"
	if overall == "watch" {
		summary = "有必需项需要留意，先别启动长测。"
	} else if overall == "unknown" {
		summary = "有必需项状态不确定，需要人工确认。"
	}
	checks = append(checks, PreflightCheck{
		ID:       "soak_gate",
		Section:  "gate",
		Label:    "长测闸门",
		Status:   overall,
		Required: true,
		Detail:   summary,
	})

	writeJSON(w, http.StatusOK, PreflightResponse{
		GeneratedAt: now.Format(time.RFC3339),
		Overall:     overall,
		Summary:     summary,
		Checks:      checks,
	})
}

func (s *Server) readinessCheck(id, section, label string, read func() (map[string]any, error)) PreflightCheck {
	data, err := read()
	if err != nil {
		return PreflightCheck{
			ID:       id,
			Section:  section,
			Label:    label,
			Status:   "watch",
			Required: true,
			Detail:   "读取失败，查看 consoleapi 日志。",
		}
	}
	return PreflightCheck{
		ID:       id,
		Section:  section,
		Label:    label,
		Status:   "good",
		Required: true,
		Detail:   "可读取。",
		Data:     data,
	}
}

func preflightOverall(checks []PreflightCheck) string {
	overall := "good"
	for _, check := range checks {
		if !check.Required {
			continue
		}
		switch check.Status {
		case "watch":
			return "watch"
		case "unknown":
			overall = "unknown"
		}
	}
	return overall
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 20)
	runs, err := s.orchestrator.ListRuns(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "runs_unavailable", err)
		return
	}
	budget, err := s.broker.RunBudgetStatus(nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "budget_unavailable", err)
		return
	}
	inbox, err := s.world.ReadHostInboxSummary(limit, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "inbox_unavailable", err)
		return
	}
	followups, err := s.world.ReadHostFollowups(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "followups_unavailable", err)
		return
	}
	inspect, _ := s.broker.RunHostInspectSummary(limit)
	acceptance, _ := s.broker.RunV0Acceptance(limit)
	var latest *orchestrator.RunRecord
	var active *orchestrator.RunStatus
	var staleRunAlerts []OperatorAlert
	if len(runs) > 0 {
		latest = &runs[0]
		active, staleRunAlerts = s.selectActiveRun(runs)
	}
	alerts := append([]OperatorAlert{}, staleRunAlerts...)
	alerts = append(alerts, buildAlerts(active, budget, inbox)...)
	out := DashboardSummary{
		GeneratedAt: s.now().Format(time.RFC3339),
		ActiveRun:   active,
		LatestRun:   latest,
		Runs:        runs,
		Budget:      budget,
		Inbox:       inbox,
		Followups:   followups,
		Alerts:      alerts,
	}
	if inspect.CollectedAt != "" {
		out.Inspect = &inspect
	}
	if acceptance.GeneratedAt != "" {
		out.Acceptance = &acceptance
	}
	writeJSON(w, http.StatusOK, out)
}

const activeRunFreshnessLimit = 30 * time.Minute

func (s *Server) selectActiveRun(runs []orchestrator.RunRecord) (*orchestrator.RunStatus, []OperatorAlert) {
	alerts := make([]OperatorAlert, 0)
	for _, run := range runs {
		if run.Status != "running" && run.Status != "paused" {
			continue
		}
		status, err := s.orchestrator.ReadRunStatus(run.RunID)
		if err != nil {
			continue
		}
		if isActiveRunFresh(status, s.now(), activeRunFreshnessLimit) {
			return &status, alerts
		}
		if alert := runFreshnessAlertAt(status, s.now(), 3*time.Minute); alert != nil {
			alerts = append(alerts, *alert)
		}
	}
	return nil, alerts
}

func isActiveRunFresh(status orchestrator.RunStatus, now time.Time, limit time.Duration) bool {
	timestamp := strings.TrimSpace(status.UpdatedAt)
	if timestamp == "" {
		timestamp = strings.TrimSpace(status.StartedAt)
	}
	if timestamp == "" {
		return true
	}
	updatedAt, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return true
	}
	return now.Sub(updatedAt) <= limit
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	out, err := s.orchestrator.ListRuns(queryInt(r, "limit", 20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "runs_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRunStatus(w http.ResponseWriter, r *http.Request) {
	out, err := s.orchestrator.ReadRunStatus(r.PathValue("runID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "run_status_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRunSummary(w http.ResponseWriter, r *http.Request) {
	out, err := s.orchestrator.ReadRunSummary(r.PathValue("runID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "run_summary_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRunReport(w http.ResponseWriter, r *http.Request) {
	out, err := s.orchestrator.ReadInspectionReport(r.PathValue("runID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "run_report_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCompactionDiagnostics(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	runs, err := s.orchestrator.ListRuns(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "compaction_diagnostics_unavailable", err)
		return
	}
	out := CompactionDiagnosticsResponse{
		GeneratedAt:  s.now().Format(time.RFC3339),
		Runs:         []CompactionRunDiagnostics{},
		RecentEvents: []CompactionDiagnosticsEvent{},
	}
	for _, run := range runs {
		summary, err := s.orchestrator.ReadRunSummary(run.RunID)
		if err != nil {
			continue
		}
		for _, residentRun := range summary.Runs {
			if residentRun.Report == nil || len(residentRun.Report.CompactionEvents) == 0 {
				continue
			}
			report := residentRun.Report
			item := newCompactionRunDiagnostics(summary.RunID, report.StartedAt, report.Resident)
			item.RunLabel = summary.Contract.Purpose
			if item.RunLabel == "" {
				item.RunLabel = string(summary.Contract.Mode)
			}
			cacheHits := 0
			for _, event := range report.CompactionEvents {
				item.TotalCompactions++
				item.TriggerBreakdown[string(event.TriggerReason)]++
				item.OutcomeBreakdown[string(event.Outcome)]++
				item.TotalTokensBefore += event.TokensBefore
				item.TotalTokensAfter += event.TokensAfter
				providerCostSpark, providerCostUSD, _, providerRecorded := compactionProviderCost(event)
				if providerRecorded {
					item.ProviderCostOnlySpark += providerCostSpark
					item.ProviderCostOnlyUSD += providerCostUSD
				} else {
					item.ProviderUsageMissing++
				}
				if event.CachePrefixHitOnCompactionCall {
					cacheHits++
				}
				if event.SummaryPaneTokensAfter > 0 {
					item.LatestSummaryPaneTokens = event.SummaryPaneTokensAfter
				}
				if event.TokensAfter > 0 {
					item.CurrentContextWindowTokens = event.TokensAfter
				}
				out.RecentEvents = append(out.RecentEvents, compactionDiagnosticsEvent(summary.RunID, event))
			}
			if report.SummaryPane != nil && report.SummaryPane.ApproxTokens > 0 {
				item.LatestSummaryPaneTokens = report.SummaryPane.ApproxTokens
			}
			if item.TotalCompactions > 0 {
				item.CacheHitRateOnCompactionCall = float64(cacheHits) / float64(item.TotalCompactions)
			}
			item.ProviderCostOnlySpark = roundDiagnosticsFloat(item.ProviderCostOnlySpark)
			item.ProviderCostOnlyUSD = roundDiagnosticsFloat(item.ProviderCostOnlyUSD)
			out.Runs = append(out.Runs, item)
		}
	}
	sortCompactionDiagnostics(&out)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBudget(w http.ResponseWriter, r *http.Request) {
	residents, err := s.allowedResidents(splitCSV(r.URL.Query().Get("resident")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_resident", err)
		return
	}
	out, err := s.broker.RunBudgetStatus(residents)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "budget_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 20)
	out, err := s.world.ReadHostInboxSummary(limit, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "inbox_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleFollowups(w http.ResponseWriter, r *http.Request) {
	out, err := s.world.ReadHostFollowups(queryInt(r, "limit", 20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "followups_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleTickets(w http.ResponseWriter, r *http.Request) {
	residents, err := s.allowedResidents(splitCSV(r.URL.Query().Get("resident")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_resident", err)
		return
	}
	resident := ""
	if len(residents) > 0 {
		resident = residents[0]
	}
	out, err := s.world.ReadTickets(resident, r.URL.Query().Get("status"), r.URL.Query().Get("priority"), queryInt(r, "limit", 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tickets_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleTicket(w http.ResponseWriter, r *http.Request) {
	ticketID, err := safeTicketID(r.PathValue("ticketID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_ticket_id", err)
		return
	}
	out, err := s.world.ReadTicket(ticketID)
	if err != nil {
		writeError(w, http.StatusNotFound, "ticket_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	out, err := s.world.ReadMessagesByStatus(r.PathValue("resident"), r.URL.Query().Get("status"), queryInt(r, "limit", 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "messages_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleThread(w http.ResponseWriter, r *http.Request) {
	out, err := s.world.ReadRecentForResident(r.PathValue("resident"), queryInt(r, "limit", 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "thread_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleInspectSummary(w http.ResponseWriter, r *http.Request) {
	out, err := s.broker.RunHostInspectSummary(queryInt(r, "limit", 20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "inspect_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAcceptance(w http.ResponseWriter, r *http.Request) {
	out, err := s.broker.RunV0Acceptance(queryInt(r, "limit", 20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "acceptance_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAcceptanceEvidence(w http.ResponseWriter, r *http.Request) {
	out, err := s.broker.RunV0AcceptanceEvidenceList(queryInt(r, "limit", 20), s.now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "evidence_unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleReply(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var input ReplyRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err)
		return
	}
	if strings.TrimSpace(input.MessageID) == "" {
		writeError(w, http.StatusBadRequest, "message_id_required", fmt.Errorf("message_id is required"))
		return
	}
	if err := validateWorldReply(input.Body, input.BoundaryAck); err != nil {
		writeError(w, http.StatusBadRequest, "reply_boundary_violation", err)
		return
	}
	out, err := s.actions.Reply(input.MessageID, input.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "reply_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleTicketReply(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var input TicketReplyRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err)
		return
	}
	if strings.TrimSpace(input.TicketID) == "" {
		writeError(w, http.StatusBadRequest, "ticket_id_required", fmt.Errorf("ticket_id is required"))
		return
	}
	if err := validateWorldReply(input.Body, input.BoundaryAck); err != nil {
		writeError(w, http.StatusBadRequest, "reply_boundary_violation", err)
		return
	}
	out, err := s.actions.ReplyTicket(input.TicketID, input.Body, input.Close)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ticket_reply_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func queryInt(r *http.Request, key string, fallback int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (s *Server) allowedResidents(residents []string) ([]string, error) {
	if len(residents) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(residents))
	for _, resident := range residents {
		resident = strings.TrimSpace(resident)
		if resident == "" {
			continue
		}
		if _, ok := s.broker.Binding(resident); !ok {
			return nil, fmt.Errorf("unknown resident: %s", resident)
		}
		out = append(out, resident)
	}
	return out, nil
}

func safeTicketID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if !strings.HasPrefix(id, "ticket-") {
		return "", fmt.Errorf("invalid ticket id")
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return "", fmt.Errorf("invalid ticket id")
	}
	return id, nil
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		if comma := strings.Index(forwarded, ","); comma >= 0 {
			forwarded = forwarded[:comma]
		}
		return strings.TrimSpace(forwarded)
	}
	host := r.RemoteAddr
	if colon := strings.LastIndex(host, ":"); colon > 0 {
		return host[:colon]
	}
	return host
}

type rateLimiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	clients map[string]*rateClient
	now     func() time.Time
}

type rateClient struct {
	tokens float64
	seen   time.Time
}

type accessStats struct {
	mu              sync.Mutex
	total           int
	total5xx        int
	rateLimited     int
	last5xx         time.Time
	lastRateLimited time.Time
}

type accessStatsSnapshot struct {
	total           int
	total5xx        int
	rateLimited     int
	last5xx         time.Time
	lastRateLimited time.Time
}

func (s *accessStats) record(status int, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total++
	if status >= http.StatusInternalServerError {
		s.total5xx++
		s.last5xx = now
	}
	if status == http.StatusTooManyRequests {
		s.rateLimited++
		s.lastRateLimited = now
	}
}

func (s *accessStats) snapshot() accessStatsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return accessStatsSnapshot{
		total:           s.total,
		total5xx:        s.total5xx,
		rateLimited:     s.rateLimited,
		last5xx:         s.last5xx,
		lastRateLimited: s.lastRateLimited,
	}
}

func newRateLimiter(ratePerSecond, burst int) *rateLimiter {
	return &rateLimiter{
		rate:    float64(ratePerSecond),
		burst:   float64(burst),
		clients: map[string]*rateClient{},
		now:     time.Now,
	}
}

func (l *rateLimiter) allow(key string) bool {
	if key == "" {
		key = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	client := l.clients[key]
	if client == nil {
		l.clients[key] = &rateClient{tokens: l.burst - 1, seen: now}
		return true
	}
	elapsed := now.Sub(client.seen).Seconds()
	client.seen = now
	client.tokens += elapsed * l.rate
	if client.tokens > l.burst {
		client.tokens = l.burst
	}
	if client.tokens < 1 {
		return false
	}
	client.tokens--
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string, err error) {
	message := http.StatusText(status)
	if status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusForbidden {
		message = code
	}
	writeJSON(w, status, ErrorEnvelope{Error: APIError{Code: code, Message: message}})
}
