package newborn

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
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
	Error       bool
	ErrorKind   string
	RawOutput   string
}

const (
	noteTextMaxChars      = 12000
	writeNoteMaxFileBytes = 256 * 1024
	actionRawOutputMax    = 12000
)

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
	case "write_note", "note_append":
		return e.executeNoteAppend(profile, decision)
	case "note_list":
		return executeNoteList(profile)
	case "note_read":
		return executeNoteRead(profile, decision)
	case "note_replace_with_backup":
		return executeNoteReplaceWithBackup(profile, decision)
	case "note_restore_backup":
		return executeNoteRestoreBackup(profile, decision)
	case "note_summarize_or_compact":
		return executeNoteSummarizeOrCompact(profile, decision)
	case "guest_exec":
		if strings.TrimSpace(decision.Command) == "" {
			return actionError("guest_exec denied: command 是必填项", "validation_error", "")
		}
		if result, denied := validateGuestExecCommand(decision.Command); denied {
			return result
		}
		return guestCommand(profile.Instance, decision.Command, classifyGuestExecActivity(decision.Command))
	case "self_status":
		return ActionResult{Observation: e.executeSelfStatus(profile), Activity: tokenledger.ActivityStatusCheck}
	case "self_quota":
		return ActionResult{Observation: e.executeSelfQuota(profile), Activity: tokenledger.ActivityStatusCheck}
	case "talk_to_chenglin":
		if strings.TrimSpace(decision.Message) == "" {
			return ActionResult{Observation: "talk_to_chenglin denied: message 是必填项", Activity: tokenledger.ActivityLightWork}
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
		return ActionResult{Observation: "没有执行任何操作", Activity: tokenledger.ActivityStatusCheck}
	}
}

func (e *IncusActionExecutor) executeNoteAppend(profile ResidentProfile, decision AgentDecision) ActionResult {
	text := strings.TrimSpace(decision.NoteText)
	if text == "" {
		text = strings.TrimSpace(decision.MemoryText)
	}
	if text == "" {
		text = strings.TrimSpace(decision.Message)
	}
	if text == "" {
		return actionError("note_append denied: 请在 note_text 提供笔记正文；note 工具不会执行 shell 命令", "validation_error", decision.Command)
	}
	if len(text) > noteTextMaxChars {
		return actionError(fmt.Sprintf("note_append denied: note_text 超过 %d 字符", noteTextMaxChars), "validation_error", text)
	}
	result := appendGuestNote(profile.Instance, decision.NoteFile, text)
	if result.Error {
		result.ErrorKind = "note_append_failed"
	}
	return result
}

func (e *IncusActionExecutor) executeSelfStatus(profile ResidentProfile) string {
	if e.broker == nil {
		return "self_status failed: broker app 未配置"
	}
	out, err := e.broker.RunStatus(profile.Name)
	if err != nil {
		return "self_status failed: " + err.Error()
	}
	return renderResidentStatusObservation(out)
}

func (e *IncusActionExecutor) executeSelfQuota(profile ResidentProfile) string {
	if e.broker == nil {
		return "self_quota failed: broker app 未配置"
	}
	out, err := e.broker.RunQuota(profile.Name)
	if err != nil {
		return "self_quota failed: " + err.Error()
	}
	return renderQuotaObservation(out)
}

func renderResidentStatusObservation(status brokerstate.ResidentStatus) string {
	lines := []string{
		"self status 快照:",
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
		"self quota 快照:",
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
		return "memory_review denied: memory_id 是必填项"
	}
	if strings.TrimSpace(decision.MemoryAction) == "" {
		return "memory_review denied: memory_action 是必填项"
	}
	updated, err := e.memories.ReviewAbstractMemory(profile.Name, decision.MemoryID, time.Now().UTC(), decision.MemoryReviewRequest())
	if err != nil {
		return "memory_review failed: " + err.Error()
	}
	return fmt.Sprintf("memory review 已应用:\nmemory_id=%s\naction=%s\nstatus=%s\nlayer=%s\nreview_state=%s\nsummary=%s",
		updated.ID,
		decision.MemoryAction,
		updated.Status,
		updated.Layer,
		updated.Governance.ReviewState,
		oneLine(updated.EffectiveSummary()),
	)
}

func suppressDuplicateAction(profile ResidentProfile, decision AgentDecision) (bool, string) {
	switch {
	case decision.NextAction == "talk_to_chenglin":
		if repeatedChat(profile.Name, decision.Message) {
			return true, "duplicate action suppressed: 最近 world thread 里已经有非常相似的聊天消息；等新事实出现，或发送语义上不同的消息"
		}
	case decision.NextAction == "submit_ticket":
		if repeatedTicket(profile.Name, decision.TicketTitle, decision.TicketBody, decision.TicketPriority) {
			return true, "duplicate action suppressed: 已存在非常相似的 ticket；用新证据更新情况，而不是重复开启同一个请求"
		}
	}
	return false, ""
}

func classifyCommandIntent(decision AgentDecision) string {
	command := normalizeDuplicateText(decision.Command)
	switch {
	case decision.NextAction == "write_note", isNoteAction(decision.NextAction):
		return "note"
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

func validateGuestExecCommand(command string) (ActionResult, bool) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return actionError("guest_exec denied: command 是必填项", "validation_error", command), true
	}
	if targetsContinuitySurface(strings.ToLower(trimmed)) {
		return actionError(
			"guest_exec denied: /root/arena-notes 下的连续性文件必须使用专用 note API：note_list、note_read、note_append、note_replace_with_backup、note_restore_backup 或 note_summarize_or_compact。这是语义上的工具选择错误，不是 shell 引号问题。",
			"continuity_surface_requires_note_tool",
			command,
		), true
	}
	return ActionResult{}, false
}

func targetsContinuitySurface(command string) bool {
	for _, token := range []string{
		"/root/arena-notes",
		"arena-notes/",
		"boot-notes.md",
	} {
		if strings.Contains(command, token) {
			return true
		}
	}
	return false
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

func guestCommand(instance, script string, activity tokenledger.ActivityType) ActionResult {
	cmd := exec.Command("incus", "exec", instance, "--", "bash", "-lc", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		raw := limitRawOutput(strings.TrimSpace(string(out)))
		return ActionResult{
			Observation: fmt.Sprintf("guest command failed:\n%s", raw),
			Activity:    activity,
			Error:       true,
			ErrorKind:   "guest_command_failed",
			RawOutput:   raw,
		}
	}
	return ActionResult{Observation: string(out), Activity: activity}
}

func executeNoteList(profile ResidentProfile) ActionResult {
	script := strings.Join([]string{
		"set -euo pipefail",
		"note_dir=/root/arena-notes",
		"mkdir -p \"$note_dir\"",
		"find \"$note_dir\" -maxdepth 1 -type f -printf '%f %s %TY-%Tm-%Td %TH:%TM\\n' | sort",
	}, "\n")
	cmd := exec.Command("incus", "exec", profile.Instance, "--", "bash", "-lc", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		raw := limitRawOutput(strings.TrimSpace(string(out)))
		return ActionResult{Observation: "note_list failed:\n" + raw, Activity: tokenledger.ActivityLightWork, Error: true, ErrorKind: "note_list_failed", RawOutput: raw}
	}
	return ActionResult{Observation: "note_list 列出 /root/arena-notes 下的文件:\n" + string(out), Activity: tokenledger.ActivityLightWork}
}

func executeNoteRead(profile ResidentProfile, decision AgentDecision) ActionResult {
	file, err := safeNoteFile(decision.NoteFile)
	if err != nil {
		return actionError("note_read denied: "+err.Error(), "note_path_invalid", decision.NoteFile)
	}
	script := strings.Join([]string{
		"set -euo pipefail",
		"note_dir=/root/arena-notes",
		"note_file=$note_dir/$1",
		"if [ ! -f \"$note_file\" ]; then echo \"note_read denied: file not found: $1\" >&2; exit 44; fi",
		"wc -c \"$note_file\"",
		"sed -n '1,220p' \"$note_file\"",
	}, "\n")
	cmd := exec.Command("incus", "exec", profile.Instance, "--", "bash", "-lc", script, "note_read", file)
	out, err := cmd.CombinedOutput()
	if err != nil {
		raw := limitRawOutput(strings.TrimSpace(string(out)))
		return ActionResult{Observation: "note_read failed:\n" + raw, Activity: tokenledger.ActivityLightWork, Error: true, ErrorKind: "note_read_failed", RawOutput: raw}
	}
	return ActionResult{Observation: "note_read 读取 /root/arena-notes/" + file + ":\n" + string(out), Activity: tokenledger.ActivityLightWork}
}

func appendGuestNote(instance, noteFile, text string) ActionResult {
	file, err := safeNoteFile(noteFile)
	if err != nil {
		return actionError("note_append denied: "+err.Error(), "note_path_invalid", noteFile)
	}
	script := strings.Join([]string{
		"set -euo pipefail",
		"note_dir=/root/arena-notes",
		"note_file=$note_dir/$1",
		"mkdir -p \"$note_dir\"",
		"current_bytes=0",
		"if [ -f \"$note_file\" ]; then current_bytes=$(wc -c < \"$note_file\"); fi",
		fmt.Sprintf("if [ \"$current_bytes\" -gt %d ]; then echo \"note_append denied: note file exceeds %d bytes\" >&2; exit 42; fi", writeNoteMaxFileBytes, writeNoteMaxFileBytes),
		"printf '%s\n' \"$2\" >> \"$note_file\"",
		"wc -c \"$note_file\"",
	}, "\n")
	cmd := exec.Command("incus", "exec", instance, "--", "bash", "-lc", script, "note_append", file, text)
	out, err := cmd.CombinedOutput()
	if err != nil {
		raw := limitRawOutput(strings.TrimSpace(string(out)))
		return ActionResult{
			Observation: fmt.Sprintf("note_append failed:\n%s", raw),
			Activity:    tokenledger.ActivityLightWork,
			Error:       true,
			ErrorKind:   "note_append_failed",
			RawOutput:   raw,
		}
	}
	return ActionResult{Observation: "note_append 已向 /root/arena-notes/" + file + " 追加纯文本\n" + string(out), Activity: tokenledger.ActivityLightWork}
}

func executeNoteReplaceWithBackup(profile ResidentProfile, decision AgentDecision) ActionResult {
	return replaceGuestNoteWithBackup(profile, decision, "note_replace_with_backup")
}

func executeNoteSummarizeOrCompact(profile ResidentProfile, decision AgentDecision) ActionResult {
	return replaceGuestNoteWithBackup(profile, decision, "note_summarize_or_compact")
}

func replaceGuestNoteWithBackup(profile ResidentProfile, decision AgentDecision, action string) ActionResult {
	file, err := safeNoteFile(decision.NoteFile)
	if err != nil {
		return actionError(action+" denied: "+err.Error(), "note_path_invalid", decision.NoteFile)
	}
	text := strings.TrimSpace(decision.NoteText)
	if text == "" {
		return actionError(action+" denied: note_text 是必填项", "validation_error", "")
	}
	if len(text) > noteTextMaxChars {
		return actionError(fmt.Sprintf("%s denied: note_text 超过 %d 字符", action, noteTextMaxChars), "validation_error", text)
	}
	script := strings.Join([]string{
		"set -euo pipefail",
		"note_dir=/root/arena-notes",
		"note_file=$note_dir/$1",
		"mkdir -p \"$note_dir\"",
		"stamp=$(date -u +%Y%m%dT%H%M%SZ)",
		"base_name=$(basename \"$note_file\")",
		"backup_file=$(mktemp \"$note_dir/$base_name.bak-$stamp.XXXXXX\")",
		"if [ -f \"$note_file\" ]; then cp \"$note_file\" \"$backup_file\"; else rm -f \"$backup_file\"; backup_file=; fi",
		"tmp_file=$(mktemp \"$note_dir/.note-replace.XXXXXX\")",
		"printf '%s\n' \"$2\" > \"$tmp_file\"",
		"mv \"$tmp_file\" \"$note_file\"",
		"if [ -n \"$backup_file\" ]; then backup_name=$(basename \"$backup_file\"); printf 'backup=%s\n' \"$backup_name\"; else printf 'backup=none\n'; fi",
		"wc -c \"$note_file\"",
	}, "\n")
	cmd := exec.Command("incus", "exec", profile.Instance, "--", "bash", "-lc", script, action, file, text)
	out, err := cmd.CombinedOutput()
	if err != nil {
		raw := limitRawOutput(strings.TrimSpace(string(out)))
		return ActionResult{Observation: action + " failed:\n" + raw, Activity: tokenledger.ActivityLightWork, Error: true, ErrorKind: "note_replace_failed", RawOutput: raw}
	}
	return ActionResult{Observation: action + " 已替换 /root/arena-notes/" + file + "\n" + string(out), Activity: tokenledger.ActivityLightWork}
}

func executeNoteRestoreBackup(profile ResidentProfile, decision AgentDecision) ActionResult {
	file, err := safeNoteFile(decision.NoteFile)
	if err != nil {
		return actionError("note_restore_backup denied: "+err.Error(), "note_path_invalid", decision.NoteFile)
	}
	backup, err := safeNoteFile(decision.BackupFile)
	if err != nil {
		return actionError("note_restore_backup denied: backup_file 无效: "+err.Error(), "note_path_invalid", decision.BackupFile)
	}
	script := strings.Join([]string{
		"set -euo pipefail",
		"note_dir=/root/arena-notes",
		"note_file=$note_dir/$1",
		"backup_file=$note_dir/$2",
		"if [ ! -f \"$backup_file\" ]; then echo \"note_restore_backup denied: backup not found: $2\" >&2; exit 45; fi",
		"stamp=$(date -u +%Y%m%dT%H%M%SZ)",
		"if [ -f \"$note_file\" ]; then cp \"$note_file\" \"$note_file.pre-restore-$stamp\"; fi",
		"cp \"$backup_file\" \"$note_file\"",
		"wc -c \"$note_file\"",
	}, "\n")
	cmd := exec.Command("incus", "exec", profile.Instance, "--", "bash", "-lc", script, "note_restore", file, backup)
	out, err := cmd.CombinedOutput()
	if err != nil {
		raw := limitRawOutput(strings.TrimSpace(string(out)))
		return ActionResult{Observation: "note_restore_backup failed:\n" + raw, Activity: tokenledger.ActivityLightWork, Error: true, ErrorKind: "note_restore_failed", RawOutput: raw}
	}
	return ActionResult{Observation: "note_restore_backup 已从 " + backup + " 恢复 /root/arena-notes/" + file + "\n" + string(out), Activity: tokenledger.ActivityLightWork}
}

func safeNoteFile(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "boot-notes.md", nil
	}
	if value == "" || strings.Contains(value, "/") || strings.Contains(value, `\`) {
		return "", fmt.Errorf("note_file 必须是 /root/arena-notes 下的文件名")
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("note_file 不能跨目录")
	}
	return cleaned, nil
}

func limitRawOutput(raw string) string {
	if len(raw) <= actionRawOutputMax {
		return raw
	}
	omitted := len(raw) - actionRawOutputMax
	return raw[:actionRawOutputMax] + "\n[raw_output_truncated bytes_omitted=" + strconv.Itoa(omitted) + "]"
}

func actionError(observation, kind, raw string) ActionResult {
	return ActionResult{
		Observation: observation,
		Activity:    tokenledger.ActivityStatusCheck,
		Error:       true,
		ErrorKind:   kind,
		RawOutput:   limitRawOutput(raw),
	}
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
	case "guest_exec":
		return decision.NextAction + ":" + normalize(decision.Command)
	case "write_note":
		return "note_append:" + normalize(firstNonEmpty(decision.NoteText, decision.MemoryText, decision.Message))
	case "note_append", "note_replace_with_backup", "note_summarize_or_compact":
		return decision.NextAction + ":" + normalize(decision.NoteFile+" "+decision.NoteText)
	case "note_read":
		return decision.NextAction + ":" + normalize(decision.NoteFile)
	case "note_restore_backup":
		return decision.NextAction + ":" + normalize(decision.NoteFile+" "+decision.BackupFile)
	case "note_list":
		return decision.NextAction
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func appendRecentAction(actions []RecentAction, item RecentAction) []RecentAction {
	actions = append(actions, item)
	if len(actions) > 6 {
		actions = actions[len(actions)-6:]
	}
	return actions
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
