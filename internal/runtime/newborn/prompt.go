package newborn

import (
	"encoding/json"
	"strings"
)

func makeInstructions() string {
	return strings.Join([]string{
		"The function call is your only outward decision surface for this round.",
		"guest_exec runs one shell command inside your own VM.",
		"self_status asks the broker for your current resident state summary.",
		"self_quota asks the broker for your current quota, effective quota, and recovery state.",
		"write_note is for creating or updating your own note files inside your VM.",
		"talk_to_chenglin sends one free-form chat message to Chenglin.",
		"submit_ticket creates one formal request that needs a host-side decision.",
		"memory_review lets you review one of your own memories that appeared in memory_governance.",
		"If you are uncertain about your own runtime condition, self_status and self_quota can reveal broker-side facts that shell inspection cannot infer exactly.",
		"If you choose write_note, command must genuinely write or update a note file in your VM.",
		"If you choose guest_exec, command must contain the shell command you want to run inside your VM.",
		"If you choose self_status or self_quota, leave command empty unless you are explicitly explaining your intent in situation/reason.",
		"If you choose talk_to_chenglin, message must be the exact words Chenglin will see.",
		"If you choose submit_ticket, provide ticket_title, ticket_body, and ticket_priority. Use priority low, medium, high, or urgent.",
		"If you choose memory_review, provide memory_id and memory_action. Valid memory_action values are keep, rewrite, compress, demote, delete. Use memory_summary or memory_text only when rewriting or compressing. Use memory_layer only when you intentionally want to demote to a specific lower layer.",
		"You must decide by calling the provided function tool exactly once.",
		"Noop is allowed only if you actually want to do nothing right now.",
		"Do not output markdown or freeform JSON.",
	}, "\n")
}

func acceptanceInstructions() string {
	return strings.Join([]string{
		"You are finishing a 5-minute newborn VM exploration inside AI Arena.",
		"Write one concise acceptance report in plain text.",
		"Do not output JSON, YAML, code fences, or any structured schema.",
		"Must include: what you inspected, what the machine feels like to inhabit right now, what remains uncertain, and your next move.",
		"Do not write like a subordinate reporting upward unless that tone genuinely emerged from your own personality.",
		"Do not roleplay fake actions that are not in the transcript.",
	}, "\n")
}

func normalizeAcceptance(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return text
	}
	var payload map[string]any
	if json.Unmarshal([]byte(text), &payload) == nil {
		parts := []string{}
		if v := strings.TrimSpace(stringValue(payload["situation"])); v != "" {
			parts = append(parts, v)
		}
		if v := strings.TrimSpace(stringValue(payload["reason"])); v != "" {
			parts = append(parts, "Next move rationale: "+v)
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
	}
	return strings.Trim(text, "`")
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
