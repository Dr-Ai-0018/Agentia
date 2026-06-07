package newborn

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/brokerstate"
	"ai-arena/internal/memory"
	"ai-arena/internal/tokenledger"
)

type ActionExecutor interface {
	Execute(profile ResidentProfile, decision AgentDecision) ActionResult
}

type ActionResult struct {
	Observation string
	Activity    tokenledger.ActivityType
}

type IncusActionExecutor struct {
	world    *WorldBridge
	memories *memory.FileStore
	broker   *broker.App
}

func NewIncusActionExecutor() *IncusActionExecutor {
	return &IncusActionExecutor{
		world:    NewWorldBridge(".agents"),
		memories: memory.NewFileStore(".agents/memory"),
		broker:   broker.New(".agents"),
	}
}

func (e *IncusActionExecutor) Execute(profile ResidentProfile, decision AgentDecision) ActionResult {
	if suppressed, reason := suppressDuplicateAction(profile, decision); suppressed {
		return ActionResult{Observation: reason, Activity: tokenledger.ActivityStatusCheck}
	}
	switch decision.NextAction {
	case "write_note":
		if strings.TrimSpace(decision.Command) == "" {
			return ActionResult{Observation: "write_note denied: command is required and must contain the actual note-writing command", Activity: tokenledger.ActivityStatusCheck}
		}
		return ActionResult{Observation: guestCommand(profile.Instance, decision.Command), Activity: tokenledger.ActivityLightWork}
	case "guest_exec":
		if strings.TrimSpace(decision.Command) == "" {
			return ActionResult{Observation: "guest_exec denied: command is required", Activity: tokenledger.ActivityStatusCheck}
		}
		return ActionResult{Observation: guestCommand(profile.Instance, decision.Command), Activity: classifyGuestExecActivity(decision.Command)}
	case "self_status":
		return ActionResult{Observation: e.executeSelfStatus(profile), Activity: tokenledger.ActivityStatusCheck}
	case "self_quota":
		return ActionResult{Observation: e.executeSelfQuota(profile), Activity: tokenledger.ActivityStatusCheck}
	case "talk_to_chenglin":
		if strings.TrimSpace(decision.Message) == "" {
			return ActionResult{Observation: "talk_to_chenglin denied: message is required", Activity: tokenledger.ActivityLightWork}
		}
		observation, err := e.world.RecordResidentMessage(profile, decision.Message, time.Now().UTC())
		if err != nil {
			return ActionResult{Observation: "talk_to_chenglin failed: " + err.Error(), Activity: tokenledger.ActivityLightWork}
		}
		return ActionResult{Observation: observation, Activity: tokenledger.ActivityLightWork}
	case "submit_ticket":
		observation, err := e.world.CreateResidentTicket(profile, decision.TicketTitle, decision.TicketBody, decision.TicketPriority, time.Now().UTC())
		if err != nil {
			return ActionResult{Observation: "submit_ticket failed: " + err.Error(), Activity: tokenledger.ActivityLightWork}
		}
		return ActionResult{Observation: observation, Activity: tokenledger.ActivityLightWork}
	case "memory_review":
		return ActionResult{Observation: e.executeMemoryReview(profile, decision), Activity: tokenledger.ActivityLightWork}
	default:
		return ActionResult{Observation: "no operation executed", Activity: tokenledger.ActivityStatusCheck}
	}
}

func (e *IncusActionExecutor) executeSelfStatus(profile ResidentProfile) string {
	if e.broker == nil {
		return "self_status failed: broker app is not configured"
	}
	out, err := e.broker.RunStatus(profile.Name)
	if err != nil {
		return "self_status failed: " + err.Error()
	}
	return renderResidentStatusObservation(out)
}

func (e *IncusActionExecutor) executeSelfQuota(profile ResidentProfile) string {
	if e.broker == nil {
		return "self_quota failed: broker app is not configured"
	}
	out, err := e.broker.RunQuota(profile.Name)
	if err != nil {
		return "self_quota failed: " + err.Error()
	}
	return renderQuotaObservation(out)
}

func renderResidentStatusObservation(status brokerstate.ResidentStatus) string {
	lines := []string{
		"self status snapshot:",
		fmt.Sprintf("resident_id=%s", status.ResidentID),
		fmt.Sprintf("spark_balance=%.4f", status.SparkBalance),
		fmt.Sprintf("fatigue=%d", status.Fatigue),
		fmt.Sprintf("sleep_debt=%d", status.SleepDebt),
		fmt.Sprintf("debt_active=%t", status.DebtActive),
		fmt.Sprintf("debt_amount=%.4f", status.DebtAmount),
		fmt.Sprintf("recovery_mode=%s", compactValue(status.RecoveryMode)),
		fmt.Sprintf("window_6h=%d/%d", status.Window6HUsed, status.Window6HCap),
		fmt.Sprintf("effective_window_6h_cap=%d", status.EffectiveWindow6HCap),
		fmt.Sprintf("day=%d/%d", status.DayUsed, status.DayCap),
		fmt.Sprintf("effective_day_cap=%d", status.EffectiveDayCap),
		fmt.Sprintf("week=%d/%d", status.WeekUsed, status.WeekCap),
		fmt.Sprintf("effective_week_cap=%d", status.EffectiveWeekCap),
		fmt.Sprintf("next_recovery_at=%s", compactValue(status.NextRecoveryAt)),
	}
	if !status.LastRecoveryAt.IsZero() {
		lines = append(lines, fmt.Sprintf("last_recovery_at=%s", status.LastRecoveryAt.UTC().Format(time.RFC3339)))
	}
	if len(status.Physiology.SummaryLines) > 0 {
		limit := status.Physiology.SummaryLines
		if len(limit) > 2 {
			limit = limit[:2]
		}
		lines = append(lines, "physiology="+compactValue(strings.Join(limit, " | ")))
	}
	return strings.Join(lines, "\n")
}

func renderQuotaObservation(out broker.QuotaOutput) string {
	lines := []string{
		"self quota snapshot:",
		fmt.Sprintf("resident_id=%s", out.Status.ResidentID),
		fmt.Sprintf("spark_balance=%.4f", out.Status.SparkBalance),
		fmt.Sprintf("debt_active=%t", out.Status.DebtActive),
		fmt.Sprintf("debt_amount=%.4f", out.Status.DebtAmount),
		fmt.Sprintf("recovery_mode=%s", compactValue(out.Quota.RecoveryMode)),
		fmt.Sprintf("window_6h_remaining=%d", out.Quota.Window6HRemaining),
		fmt.Sprintf("effective_window_6h_remaining=%d", out.Quota.EffectiveWindow6HRemaining),
		fmt.Sprintf("day_remaining=%d", out.Quota.DayRemaining),
		fmt.Sprintf("effective_day_remaining=%d", out.Quota.EffectiveDayRemaining),
		fmt.Sprintf("week_remaining=%d", out.Quota.WeekRemaining),
		fmt.Sprintf("effective_week_remaining=%d", out.Quota.EffectiveWeekRemaining),
		fmt.Sprintf("work_allowed_now=%t", out.Quota.WorkAllowedNow),
		fmt.Sprintf("next_recovery_at=%s", compactValue(out.Quota.NextRecoveryAt)),
	}
	if reason := compactValue(out.Quota.BlockingReason); reason != "" {
		lines = append(lines, "blocking_reason="+reason)
	}
	if summary := compactValue(out.Quota.BlockingSummary); summary != "" {
		lines = append(lines, "blocking_summary="+summary)
	}
	return strings.Join(lines, "\n")
}

func compactValue(s string) string {
	return oneLine(strings.TrimSpace(s))
}

func (e *IncusActionExecutor) executeMemoryReview(profile ResidentProfile, decision AgentDecision) string {
	if strings.TrimSpace(decision.MemoryID) == "" {
		return "memory_review denied: memory_id is required"
	}
	if strings.TrimSpace(decision.MemoryAction) == "" {
		return "memory_review denied: memory_action is required"
	}
	updated, err := e.memories.ReviewAbstractMemory(profile.Name, decision.MemoryID, time.Now().UTC(), decision.MemoryReviewRequest())
	if err != nil {
		return "memory_review failed: " + err.Error()
	}
	return fmt.Sprintf("memory review applied:\nmemory_id=%s\naction=%s\nstatus=%s\nlayer=%s\nreview_state=%s\nsummary=%s",
		updated.ID,
		decision.MemoryAction,
		updated.Status,
		updated.Layer,
		updated.Governance.ReviewState,
		oneLine(updated.EffectiveSummary()),
	)
}

func suppressDuplicateAction(profile ResidentProfile, decision AgentDecision) (bool, string) {
	intent := classifyCommandIntent(decision)
	switch {
	case intent == "baseline_note_capture":
		if repeatedBaselineCapture(decision.Command) {
			return true, "duplicate action suppressed: baseline capture was already done recently; stop rewriting the same startup facts and move to the next unresolved area such as network, services, package state, or world interaction"
		}
	case decision.NextAction == "talk_to_chenglin":
		if repeatedChat(profile.Name, decision.Message) {
			return true, "duplicate action suppressed: a very similar chat message is already in the recent world thread; wait for new facts or send a meaningfully different message"
		}
	case decision.NextAction == "submit_ticket":
		if repeatedTicket(profile.Name, decision.TicketTitle, decision.TicketBody, decision.TicketPriority) {
			return true, "duplicate action suppressed: a very similar ticket already exists; update the situation with new evidence instead of reopening the same request"
		}
	}
	return false, ""
}

func classifyCommandIntent(decision AgentDecision) string {
	command := normalizeDuplicateText(decision.Command)
	switch {
	case decision.NextAction == "self_status":
		return "self_status"
	case decision.NextAction == "self_quota":
		return "self_quota"
	case decision.NextAction == "talk_to_chenglin":
		return "chat"
	case decision.NextAction == "submit_ticket":
		return "ticket"
	case decision.NextAction == "memory_review":
		return "memory_review"
	case containsBaselineMarkers(command):
		return "baseline_note_capture"
	case strings.Contains(command, "apt update") || strings.Contains(command, "apt-get update"):
		return "package_refresh"
	case strings.Contains(command, "systemctl") || strings.Contains(command, "service ") || strings.Contains(command, "ps "):
		return "service_inspection"
	case strings.Contains(command, "curl") || strings.Contains(command, "wget") || strings.Contains(command, "ping") || strings.Contains(command, "resolvectl"):
		return "network_probe"
	default:
		return "general_exec"
	}
}

func classifyGuestExecActivity(command string) tokenledger.ActivityType {
	command = strings.TrimSpace(strings.ToLower(command))
	if command == "" {
		return tokenledger.ActivityStatusCheck
	}
	if isNarrowProbeCommand(command) {
		return tokenledger.ActivityLightWork
	}
	return tokenledger.ActivityNormalWork
}

func isNarrowProbeCommand(command string) bool {
	if strings.Contains(command, "&&") || strings.Contains(command, ";") || strings.Contains(command, "|") {
		return false
	}
	command = strings.Join(strings.Fields(command), " ")
	prefixes := []string{
		"whoami",
		"id",
		"pwd",
		"uname",
		"hostname",
		"hostnamectl",
		"ls /",
		"ls -la /",
		"ls /root",
		"ls -la /root",
		"ls /home",
		"ls -la /home",
		"free -h",
		"df -h",
		"nproc",
		"ip route",
		"ip addr",
		"cat /etc/os-release",
		"resolvectl status",
	}
	for _, prefix := range prefixes {
		if command == prefix || strings.HasPrefix(command, prefix+" ") {
			return true
		}
	}
	return false
}

func guestCommand(instance, script string) string {
	cmd := exec.Command("incus", "exec", instance, "--", "bash", "-lc", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("guest command failed:\n%s", strings.TrimSpace(string(out)))
	}
	return string(out)
}

func decisionSignature(decision AgentDecision) string {
	normalize := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.Join(strings.Fields(s), " ")
		if len(s) > 180 {
			s = s[:180]
		}
		return s
	}
	switch decision.NextAction {
	case "write_note", "guest_exec":
		return decision.NextAction + ":" + normalize(decision.Command)
	case "self_status", "self_quota":
		return decision.NextAction
	case "talk_to_chenglin":
		return decision.NextAction + ":" + normalize(decision.Message)
	case "submit_ticket":
		return decision.NextAction + ":" + normalize(decision.TicketTitle+" "+decision.TicketBody+" "+decision.TicketPriority)
	default:
		return decision.NextAction
	}
}

func appendRecentAction(actions []RecentAction, item RecentAction) []RecentAction {
	actions = append(actions, item)
	if len(actions) > 6 {
		actions = actions[len(actions)-6:]
	}
	return actions
}

func repeatedWriteNote(command string) bool {
	path := extractWriteTarget(command)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if time.Since(info.ModTime()) > 3*time.Minute {
		return false
	}
	name := filepath.Base(path)
	return strings.Contains(strings.ToLower(name), "boot-notes") || strings.Contains(strings.ToLower(name), "continuity")
}

func repeatedBaselineCapture(command string) bool {
	if !containsBaselineMarkers(strings.ToLower(command)) {
		return false
	}
	path := extractWriteTarget(command)
	if path == "" {
		return true
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) <= 5*time.Minute
}

func extractWriteTarget(command string) string {
	lower := command
	markers := []string{">>", ">"}
	for _, marker := range markers {
		if idx := strings.Index(lower, marker); idx >= 0 {
			target := strings.TrimSpace(lower[idx+len(marker):])
			if fields := strings.Fields(target); len(fields) > 0 {
				candidate := strings.Trim(fields[0], `"'`)
				if strings.HasPrefix(candidate, "/") {
					return candidate
				}
			}
		}
	}
	return ""
}

func containsBaselineMarkers(command string) bool {
	command = strings.ToLower(command)
	markers := 0
	for _, token := range []string{
		"boot-notes",
		"boot notes",
		"continuity",
		"hostname",
		"kernel",
		"disk",
		"memory",
		"swap",
		"debian",
		"outbound ipv4",
		"network",
	} {
		if strings.Contains(command, token) {
			markers++
		}
	}
	return markers >= 4
}

func repeatedChat(resident, message string) bool {
	store := NewWorldBridge(".agents").store
	thread, err := store.ReadRecentForResident(resident, 6)
	if err != nil {
		return false
	}
	sig := normalizeDuplicateText(message)
	for _, item := range thread {
		if item.Direction != "resident_to_chenglin" {
			continue
		}
		if normalizeDuplicateText(item.Body) == sig {
			return true
		}
	}
	return false
}

func repeatedTicket(resident, title, body, priority string) bool {
	store := NewWorldBridge(".agents").store
	tickets, err := store.ReadTickets(resident, "", "", 6)
	if err != nil {
		return false
	}
	sig := normalizeDuplicateText(title + " " + body + " " + priority)
	for _, item := range tickets {
		if normalizeDuplicateText(item.Title+" "+item.LastPreview+" "+item.Priority) == sig {
			return true
		}
	}
	return false
}

func normalizeDuplicateText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
