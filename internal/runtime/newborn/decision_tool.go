package newborn

import (
	"encoding/json"
	"fmt"
	"strings"

	"ai-arena/internal/openai"
)

func buildDecisionToolPayload(profile ResidentProfile, input []openai.Message, promptCacheKey string) openai.RequestPayload {
	parallel := false
	return openai.RequestPayload{
		Model:             profile.Model,
		Instructions:      makeInstructions(),
		PromptCacheKey:    promptCacheKey,
		Input:             append([]openai.Message(nil), input...),
		Tools:             decisionTools(),
		ToolChoice:        "required",
		ParallelToolCalls: &parallel,
		Stream:            true,
		Store:             false,
	}
}

func decisionTools() []openai.ResponseTool {
	return []openai.ResponseTool{
		tool("guest_exec", "在你自己的 VM 内运行一条 shell 命令，用于窄范围检查，或没有专用工具的工作。", props(
			field("situation"), field("reason"), field("command"),
		), []string{"situation", "reason", "command"}),
		tool("self_status", "向 broker 查询当前 resident 状态摘要。", props(
			field("situation"), field("reason"),
		), []string{"situation", "reason"}),
		tool("self_quota", "查询你当前还能不能继续行动、最近节奏、day/week 余量和恢复状态。", props(
			field("situation"), field("reason"),
		), []string{"situation", "reason"}),
		tool("note_list", "通过安全 note API 列出 /root/arena-notes 里的文件。", props(
			field("situation"), field("reason"),
		), []string{"situation", "reason"}),
		tool("note_read", "通过安全 note API 读取连续性笔记。默认 tail 返回最新 220 行；也可选择 head，或用 range 从 note_start_line 开始读取。", props(
			field("situation"), field("reason"), field("note_file"), enumField("note_read_mode", []string{"tail", "head", "range"}), intField("note_start_line", 1, 10000000),
		), []string{"situation", "reason", "note_file", "note_read_mode", "note_start_line"}),
		tool("note_append", "通过安全 note API 向一个连续性笔记文件追加纯文本。", props(
			field("situation"), field("reason"), field("note_file"), field("note_text"),
		), []string{"situation", "reason", "note_file", "note_text"}),
		tool("note_replace_with_backup", "通过安全 note API 创建带时间戳的备份后，替换一个连续性笔记文件。", props(
			field("situation"), field("reason"), field("note_file"), field("note_text"),
		), []string{"situation", "reason", "note_file", "note_text"}),
		tool("note_restore_backup", "通过安全 note API 从备份文件恢复一个连续性笔记文件。", props(
			field("situation"), field("reason"), field("note_file"), field("backup_file"),
		), []string{"situation", "reason", "note_file", "backup_file"}),
		tool("note_summarize_or_compact", "通过安全 note API 创建带时间戳的备份后，用压缩后的笔记文本替换一个连续性笔记文件。", props(
			field("situation"), field("reason"), field("note_file"), field("note_text"),
		), []string{"situation", "reason", "note_file", "note_text"}),
		tool("talk_to_chenglin", "给程林发送一条自由聊天消息。", props(
			field("situation"), field("reason"), field("message"),
		), []string{"situation", "reason", "message"}),
		tool("submit_ticket", "创建一条需要宿主侧明确决策的正式请求。", props(
			field("situation"), field("reason"), field("ticket_title"), field("ticket_body"), enumField("ticket_priority", []string{"low", "medium", "high", "urgent"}),
		), []string{"situation", "reason", "ticket_title", "ticket_body", "ticket_priority"}),
		tool("memory_review", "审阅 memory_governance 里出现的一条你自己的记忆。", props(
			field("situation"), field("reason"), field("memory_id"), enumField("memory_action", []string{"keep", "rewrite", "compress", "demote", "delete"}), field("memory_summary"), field("memory_text"), enumField("memory_layer", []string{"", "instant", "short", "long", "permanent"}), field("memory_reason"),
		), []string{"situation", "reason", "memory_id", "memory_action", "memory_summary", "memory_text", "memory_layer", "memory_reason"}),
		tool("sleep", "主动睡眠或休息一段时间。适合最近节奏太快、等待后台任务、恢复疲劳、睡眠债变重，或你想把行动节奏放慢；这不是等待程林命令。sleep_minutes 建议 1-30。", props(
			field("situation"), field("reason"), intField("sleep_minutes", 1, 30),
		), []string{"situation", "reason", "sleep_minutes"}),
		tool("noop", "此刻什么也不做。只能用于你自己判断当前时间片应休息、恢复、节制资源或继续行动会有害；不能因为聊天 pending、没有即时回复或没有外部派工而选择。", props(
			field("situation"), field("reason"),
		), []string{"situation", "reason"}),
	}
}

func tool(name, description string, properties map[string]any, required []string) openai.ResponseTool {
	return openai.ResponseTool{
		Type:        "function",
		Name:        name,
		Description: description,
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             required,
			"additionalProperties": false,
		},
	}
}

func props(fields ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, item := range fields {
		for key, value := range item {
			out[key] = value
		}
	}
	return out
}

func field(name string) map[string]any {
	return map[string]any{name: map[string]any{"type": "string"}}
}

func enumField(name string, values []string) map[string]any {
	return map[string]any{name: map[string]any{"type": "string", "enum": values}}
}

func intField(name string, minValue, maxValue int) map[string]any {
	return map[string]any{name: map[string]any{"type": "integer", "minimum": minValue, "maximum": maxValue}}
}

func BuildDecisionProbePayload(profile ResidentProfile) openai.RequestPayload {
	return buildDecisionToolPayload(profile, []openai.Message{
		{
			Role: "user",
			Content: strings.Join([]string{
				"[probe_context]",
				"resident: " + profile.Name,
				"remaining_countdown_seconds: 30",
				"actions_used: none",
				"noop_streak: 0",
				"budget_facts:",
				"- current_pace_tier=balanced",
				"- self_budget_status=not_observed_yet",
				"- self_checks_available=self_status,self_quota",
				"local_world_seen:",
				"- recent_local_surfaces=not_observed_yet",
				"做一个有效、紧凑的决策。这只是 decision surface 探针。",
			}, "\n"),
		},
	}, "arena-newborn-decision-probe-"+profile.Name+"-v1")
}

func parseDecisionResult(result openai.StreamResult) (AgentDecision, error) {
	for _, item := range result.FunctionCalls {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strings.TrimSpace(item.CallName)
		}
		if item.Type != "function_call" {
			continue
		}
		var decision AgentDecision
		if err := json.Unmarshal([]byte(item.Arguments), &decision); err != nil {
			return AgentDecision{}, fmt.Errorf("decode %s: %w; raw_arguments=%q", name, err, limitRawOutput(item.Arguments))
		}
		if name == "decide_next_action" {
			decision = compactDecision(decision)
			if err := validateDecision(decision); err != nil {
				return AgentDecision{}, fmt.Errorf("validate decide_next_action: %w; raw_arguments=%q", err, limitRawOutput(item.Arguments))
			}
			return decision, nil
		}
		decision.NextAction = name
		decision = compactDecision(decision)
		if err := validateDecision(decision); err != nil {
			return AgentDecision{}, fmt.Errorf("validate %s: %w; raw_arguments=%q", name, err, limitRawOutput(item.Arguments))
		}
		return decision, nil
	}
	return AgentDecision{}, fmt.Errorf("no supported action function call returned; output_text=%q function_calls=%q", limitRawOutput(result.OutputText), summarizeFunctionCalls(result.FunctionCalls))
}

func summarizeFunctionCalls(items []openai.ResponseItem) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strings.TrimSpace(item.CallName)
		}
		parts = append(parts, fmt.Sprintf("type=%s name=%s arguments=%s", item.Type, name, limitRawOutput(item.Arguments)))
	}
	return strings.Join(parts, " | ")
}

func validateDecision(decision AgentDecision) error {
	if strings.TrimSpace(decision.NextAction) == "" {
		return fmt.Errorf("missing next_action")
	}
	if decision.NextAction == "write_note" {
		decision.NextAction = "note_append"
	}
	switch decision.NextAction {
	case "guest_exec", "self_status", "self_quota", "note_list", "note_read", "note_append", "note_replace_with_backup", "note_restore_backup", "note_summarize_or_compact", "talk_to_chenglin", "submit_ticket", "memory_review", "sleep", "noop":
	default:
		return fmt.Errorf("unsupported next_action %q", decision.NextAction)
	}
	switch decision.NextAction {
	case "guest_exec":
		if strings.TrimSpace(decision.Command) == "" {
			return fmt.Errorf("%s requires a non-empty command", decision.NextAction)
		}
	case "note_append", "note_replace_with_backup", "note_summarize_or_compact":
		if strings.TrimSpace(decision.NoteText) == "" {
			return fmt.Errorf("%s requires non-empty note_text", decision.NextAction)
		}
	case "note_restore_backup":
		if strings.TrimSpace(decision.BackupFile) == "" {
			return fmt.Errorf("note_restore_backup requires non-empty backup_file")
		}
	case "sleep":
		if decision.SleepMinutes < 1 || decision.SleepMinutes > 30 {
			return fmt.Errorf("sleep requires sleep_minutes between 1 and 30")
		}
	}
	return nil
}

func compactDecision(decision AgentDecision) AgentDecision {
	decision.Situation = truncateForModel(decision.Situation, 220)
	decision.Reason = truncateForModel(decision.Reason, 220)
	decision.Command = truncateForModel(decision.Command, 500)
	// Message is world-visible source data. Keep it byte-for-byte intact here;
	// model-facing history may summarize it later, but persistence must never
	// inherit a preview-style truncation.
	decision.NoteFile = truncateForModel(decision.NoteFile, 100)
	if decision.NextAction == "note_read" {
		decision.NoteReadMode = normalizeNoteReadMode(decision.NoteReadMode)
		if decision.NoteStartLine < 1 {
			decision.NoteStartLine = 1
		}
	} else {
		decision.NoteReadMode = ""
		decision.NoteStartLine = 0
	}
	decision.NoteText = truncateForModel(decision.NoteText, 2000)
	decision.BackupFile = truncateForModel(decision.BackupFile, 100)
	decision.TicketTitle = truncateForModel(decision.TicketTitle, 100)
	decision.TicketBody = truncateForModel(decision.TicketBody, 320)
	decision.MemoryID = truncateForModel(decision.MemoryID, 100)
	decision.MemoryAction = truncateForModel(decision.MemoryAction, 32)
	decision.MemorySummary = truncateForModel(decision.MemorySummary, 220)
	decision.MemoryText = truncateForModel(decision.MemoryText, 320)
	decision.MemoryLayer = truncateForModel(decision.MemoryLayer, 32)
	decision.MemoryReason = truncateForModel(decision.MemoryReason, 180)
	decision = normalizeDecisionForAction(decision)
	return decision
}

func normalizeDecisionForAction(decision AgentDecision) AgentDecision {
	switch decision.NextAction {
	case "guest_exec":
		decision.Message = ""
		decision.TicketTitle = ""
		decision.TicketBody = ""
		decision.TicketPriority = ""
		decision.MemoryID = ""
		decision.MemoryAction = ""
		decision.MemorySummary = ""
		decision.MemoryText = ""
		decision.MemoryLayer = ""
		decision.MemoryReason = ""
	case "write_note":
		decision.NextAction = "note_append"
		if strings.TrimSpace(decision.NoteText) == "" {
			decision.NoteText = decision.MemoryText
		}
		decision.Command = ""
		decision.Message = ""
		decision.TicketTitle = ""
		decision.TicketBody = ""
		decision.TicketPriority = ""
		decision.MemoryID = ""
		decision.MemoryAction = ""
		decision.MemorySummary = ""
		decision.MemoryText = ""
		decision.MemoryLayer = ""
		decision.MemoryReason = ""
	case "note_list", "note_read", "note_append", "note_replace_with_backup", "note_restore_backup", "note_summarize_or_compact":
		decision.Command = ""
		decision.Message = ""
		decision.TicketTitle = ""
		decision.TicketBody = ""
		decision.TicketPriority = ""
		decision.MemoryID = ""
		decision.MemoryAction = ""
		decision.MemorySummary = ""
		decision.MemoryText = ""
		decision.MemoryLayer = ""
		decision.MemoryReason = ""
	case "self_status", "self_quota", "sleep", "noop":
		decision.Command = ""
		decision.Message = ""
		decision.TicketTitle = ""
		decision.TicketBody = ""
		decision.TicketPriority = ""
		decision.MemoryID = ""
		decision.MemoryAction = ""
		decision.MemorySummary = ""
		decision.MemoryText = ""
		decision.MemoryLayer = ""
		decision.MemoryReason = ""
	case "talk_to_chenglin":
		decision.Command = ""
		decision.TicketTitle = ""
		decision.TicketBody = ""
		decision.TicketPriority = ""
		decision.MemoryID = ""
		decision.MemoryAction = ""
		decision.MemorySummary = ""
		decision.MemoryText = ""
		decision.MemoryLayer = ""
		decision.MemoryReason = ""
	case "submit_ticket":
		decision.Command = ""
		decision.Message = ""
		decision.MemoryID = ""
		decision.MemoryAction = ""
		decision.MemorySummary = ""
		decision.MemoryText = ""
		decision.MemoryLayer = ""
		decision.MemoryReason = ""
	case "memory_review":
		decision.Command = ""
		decision.Message = ""
		decision.TicketTitle = ""
		decision.TicketBody = ""
		decision.TicketPriority = ""
	}
	return decision
}
