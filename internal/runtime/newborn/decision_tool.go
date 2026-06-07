package newborn

import (
	"encoding/json"
	"fmt"
	"strings"

	"ai-arena/internal/openai"
)

func buildDecisionToolPayload(profile ResidentProfile, input []openai.Message, promptCacheKey string) openai.RequestPayload {
	parallelToolCalls := false
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
							"enum": []string{"guest_exec", "write_note", "talk_to_chenglin", "submit_ticket", "memory_review", "noop"},
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
					"required":             []string{"situation", "next_action", "reason", "command", "message", "ticket_title", "ticket_body", "ticket_priority", "memory_id", "memory_action", "memory_summary", "memory_text", "memory_layer", "memory_reason"},
					"additionalProperties": false,
				},
			},
		},
		ToolChoice:        openai.FunctionToolChoice{Type: "function", Name: "decide_next_action"},
		ParallelToolCalls: &parallelToolCalls,
		Stream:            true,
		Store:             false,
	}
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
			return AgentDecision{}, fmt.Errorf("decode decide_next_action: %w", err)
		}
		if err := validateDecision(decision); err != nil {
			return AgentDecision{}, err
		}
		return decision, nil
	}
	return AgentDecision{}, fmt.Errorf("no decide_next_action function call returned; output_text=%q", result.OutputText)
}

func validateDecision(decision AgentDecision) error {
	if strings.TrimSpace(decision.NextAction) == "" {
		return fmt.Errorf("missing next_action")
	}
	switch decision.NextAction {
	case "guest_exec", "write_note", "talk_to_chenglin", "submit_ticket", "memory_review", "noop":
	default:
		return fmt.Errorf("unsupported next_action %q", decision.NextAction)
	}
	return nil
}
