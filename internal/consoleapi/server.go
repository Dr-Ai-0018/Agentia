package consoleapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/orchestrator"
	"ai-arena/internal/worldstate"
)

type Server struct {
	root         string
	broker       *broker.App
	world        *worldstate.Store
	actions      *broker.HostActionService
	orchestrator *orchestrator.Service
	now          func() time.Time
}

type Options struct {
	Root string
	Now  func() time.Time
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
		root:         root,
		broker:       app,
		world:        worldstate.New(root),
		actions:      broker.NewHostActionService(root),
		orchestrator: orchestrator.New(app, &http.Client{Timeout: 30 * time.Second}, "", ""),
		now:          now,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/summary", s.handleSummary)
	mux.HandleFunc("GET /api/runs", s.handleRuns)
	mux.HandleFunc("GET /api/runs/{runID}/status", s.handleRunStatus)
	mux.HandleFunc("GET /api/runs/{runID}/summary", s.handleRunSummary)
	mux.HandleFunc("GET /api/runs/{runID}/report", s.handleRunReport)
	mux.HandleFunc("GET /api/budget", s.handleBudget)
	mux.HandleFunc("GET /api/inbox", s.handleInbox)
	mux.HandleFunc("GET /api/followups", s.handleFollowups)
	mux.HandleFunc("GET /api/messages/{resident}", s.handleMessages)
	mux.HandleFunc("GET /api/messages/{resident}/thread", s.handleThread)
	mux.HandleFunc("GET /api/system/inspect-summary", s.handleInspectSummary)
	mux.HandleFunc("GET /api/acceptance", s.handleAcceptance)
	mux.HandleFunc("GET /api/acceptance/evidence", s.handleAcceptanceEvidence)
	mux.HandleFunc("POST /api/reply", s.handleReply)
	mux.HandleFunc("POST /api/ticket-reply", s.handleTicketReply)
	return withJSONHeaders(mux)
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
		"root":         s.root,
	})
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
	if len(runs) > 0 {
		latest = &runs[0]
		for _, run := range runs {
			if run.Status == "running" || run.Status == "paused" {
				status, err := s.orchestrator.ReadRunStatus(run.RunID)
				if err == nil {
					active = &status
					break
				}
			}
		}
	}
	out := DashboardSummary{
		GeneratedAt: s.now().Format(time.RFC3339),
		ActiveRun:   active,
		LatestRun:   latest,
		Runs:        runs,
		Budget:      budget,
		Inbox:       inbox,
		Followups:   followups,
		Alerts:      buildAlerts(active, budget, inbox),
	}
	if inspect.CollectedAt != "" {
		out.Inspect = &inspect
	}
	if acceptance.GeneratedAt != "" {
		out.Acceptance = &acceptance
	}
	writeJSON(w, http.StatusOK, out)
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

func (s *Server) handleBudget(w http.ResponseWriter, r *http.Request) {
	out, err := s.broker.RunBudgetStatus(splitCSV(r.URL.Query().Get("resident")))
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, ErrorEnvelope{Error: APIError{Code: code, Message: err.Error()}})
}
