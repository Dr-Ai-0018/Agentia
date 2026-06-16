package newborn

import (
	"encoding/json"
	"fmt"
	"strings"

	"ai-arena/internal/openai"
)

func buildDecisionToolPayload(profile ResidentProfile, input []openai.Message, promptCacheKey string) openai.RequestPayload {
	return openai.RequestPayload{
		Model:          profile.Model,
		Instructions:   makeInstructions(),
		PromptCacheKey: promptCacheKey,
		Input:          append([]openai.Message(nil), input...),
		Tools: []openai.ResponseTool{
			{
				Type:        "function",
				Name:        "decide_next_action",
				Description: "Choose exactly one next action for the resident's own VM session.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"situation": map[string]any{
							"type": "string",
						},
						"next_action": map[string]any{
							"type": "string",
							"enum": []string{"guest_exec", "self_status", "self_quota", "write_note", "talk_to_chenglin", "submit_ticket", "memory_review", "noop"},
						},
						"reason": map[string]any{
							"type": "string",
						},
						"command": map[string]any{
							"type": "string",
						},
						"message": map[string]any{
							"type": "string",
						},
						"ticket_title": map[string]any{
							"type": "string",
						},
						"ticket_body": map[string]any{
							"type": "string",
						},
						"ticket_priority": map[string]any{
							"type": "string",
							"enum": []string{"", "low", "medium", "high", "urgent"},
						},
						"memory_id": map[string]any{
							"type": "string",
						},
						"memory_action": map[string]any{
							"type": "string",
							"enum": []string{"", "keep", "rewrite", "compress", "demote", "delete"},
						},
						"memory_summary": map[string]any{
							"type": "string",
						},
						"memory_text": map[string]any{
							"type": "string",
						},
						"memory_layer": map[string]any{
							"type": "string",
							"enum": []string{"", "instant", "short", "long", "permanent"},
						},
						"memory_reason": map[string]any{
							"type": "string",
						},
					},
					"required": []string{
						"situation",
						"next_action",
						"reason",
						"command",
						"message",
						"ticket_title",
						"ticket_body",
						"ticket_priority",
						"memory_id",
						"memory_action",
						"memory_summary",
						"memory_text",
						"memory_layer",
						"memory_reason",
					},
					"additionalProperties": false,
				},
			},
		},
		Stream: true,
		Store:  false,
	}
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
		if item.Type != "function_call" || name != "decide_next_action" {
			continue
		}
		var decision AgentDecision
		if err := json.Unmarshal([]byte(item.Arguments), &decision); err != nil {
			return AgentDecision{}, fmt.Errorf("decode decide_next_action: %w; raw_arguments=%q", err, limitRawOutput(item.Arguments))
		}
		decision = compactDecision(decision)
		if err := validateDecision(decision); err != nil {
			return AgentDecision{}, fmt.Errorf("validate decide_next_action: %w; raw_arguments=%q", err, limitRawOutput(item.Arguments))
		}
		return decision, nil
	}
	return AgentDecision{}, fmt.Errorf("no decide_next_action function call returned; output_text=%q function_calls=%q", limitRawOutput(result.OutputText), summarizeFunctionCalls(result.FunctionCalls))
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
	switch decision.NextAction {
	case "guest_exec", "self_status", "self_quota", "write_note", "talk_to_chenglin", "submit_ticket", "memory_review", "noop":
	default:
		return fmt.Errorf("unsupported next_action %q", decision.NextAction)
	}
	switch decision.NextAction {
	case "guest_exec":
		if strings.TrimSpace(decision.Command) == "" {
			return fmt.Errorf("%s requires a non-empty command", decision.NextAction)
		}
	case "write_note":
		if strings.TrimSpace(decision.MemoryText) == "" && strings.TrimSpace(decision.Message) == "" {
			return fmt.Errorf("write_note requires non-empty memory_text")
		}
	}
	return nil
}

func compactDecision(decision AgentDecision) AgentDecision {
	decision.Situation = truncateForModel(decision.Situation, 220)
	decision.Reason = truncateForModel(decision.Reason, 220)
	decision.Command = truncateForModel(decision.Command, 500)
	decision.Message = truncateForModel(decision.Message, 260)
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
		decision.Command = ""
		decision.Message = ""
		decision.TicketTitle = ""
		decision.TicketBody = ""
		decision.TicketPriority = ""
		decision.MemoryID = ""
		decision.MemoryAction = ""
		decision.MemorySummary = ""
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
