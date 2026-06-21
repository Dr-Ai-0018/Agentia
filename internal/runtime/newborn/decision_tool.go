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
		tool("guest_exec", "Run one shell command inside your own VM for narrow inspection or work without a dedicated tool.", props(
			field("situation"), field("reason"), field("command"),
		), []string{"situation", "reason", "command"}),
		tool("self_status", "Ask the broker for current resident state summary.", props(
			field("situation"), field("reason"),
		), []string{"situation", "reason"}),
		tool("self_quota", "Ask the broker for current quota, effective quota, and recovery state.", props(
			field("situation"), field("reason"),
		), []string{"situation", "reason"}),
		tool("note_list", "List files in /root/arena-notes using the safe note API.", props(
			field("situation"), field("reason"),
		), []string{"situation", "reason"}),
		tool("note_read", "Read one continuity note file using the safe note API.", props(
			field("situation"), field("reason"), field("note_file"),
		), []string{"situation", "reason", "note_file"}),
		tool("note_append", "Append plain note text to one continuity note file using the safe note API.", props(
			field("situation"), field("reason"), field("note_file"), field("note_text"),
		), []string{"situation", "reason", "note_file", "note_text"}),
		tool("note_replace_with_backup", "Replace one continuity note file after creating a timestamped backup using the safe note API.", props(
			field("situation"), field("reason"), field("note_file"), field("note_text"),
		), []string{"situation", "reason", "note_file", "note_text"}),
		tool("note_restore_backup", "Restore one continuity note file from a backup file using the safe note API.", props(
			field("situation"), field("reason"), field("note_file"), field("backup_file"),
		), []string{"situation", "reason", "note_file", "backup_file"}),
		tool("note_summarize_or_compact", "Replace one continuity note file with compacted note text after creating a timestamped backup using the safe note API.", props(
			field("situation"), field("reason"), field("note_file"), field("note_text"),
		), []string{"situation", "reason", "note_file", "note_text"}),
		tool("talk_to_chenglin", "Send one free-form chat message to Chenglin.", props(
			field("situation"), field("reason"), field("message"),
		), []string{"situation", "reason", "message"}),
		tool("submit_ticket", "Create one formal request that needs a host-side decision.", props(
			field("situation"), field("reason"), field("ticket_title"), field("ticket_body"), enumField("ticket_priority", []string{"low", "medium", "high", "urgent"}),
		), []string{"situation", "reason", "ticket_title", "ticket_body", "ticket_priority"}),
		tool("memory_review", "Review one of your own memories from memory_governance.", props(
			field("situation"), field("reason"), field("memory_id"), enumField("memory_action", []string{"keep", "rewrite", "compress", "demote", "delete"}), field("memory_summary"), field("memory_text"), enumField("memory_layer", []string{"", "instant", "short", "long", "permanent"}), field("memory_reason"),
		), []string{"situation", "reason", "memory_id", "memory_action", "memory_summary", "memory_text", "memory_layer", "memory_reason"}),
		tool("noop", "Do nothing now.", props(
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
				"- budget_tier=balanced",
				"- budget_status=not_observed_yet",
				"- broker_self_surfaces_available=self_status,self_quota",
				"exploration_frontier:",
				"- next_preferred_surface=identity",
				"- next_probe_shape=single identity probe such as whoami or hostname",
				"Make one valid compact decision. This is only a decision-surface probe.",
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
	case "guest_exec", "self_status", "self_quota", "note_list", "note_read", "note_append", "note_replace_with_backup", "note_restore_backup", "note_summarize_or_compact", "talk_to_chenglin", "submit_ticket", "memory_review", "noop":
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
	}
	return nil
}

func compactDecision(decision AgentDecision) AgentDecision {
	decision.Situation = truncateForModel(decision.Situation, 220)
	decision.Reason = truncateForModel(decision.Reason, 220)
	decision.Command = truncateForModel(decision.Command, 500)
	decision.Message = truncateForModel(decision.Message, 260)
	decision.NoteFile = truncateForModel(decision.NoteFile, 100)
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
	case "self_status", "self_quota", "noop":
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
