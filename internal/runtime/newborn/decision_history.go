package newborn

import (
	"strings"

	"ai-arena/internal/context"
	"ai-arena/internal/openai"
)

func buildDecisionInput(stablePrefix string, history []openai.Message) []openai.Message {
	input := make([]openai.Message, 0, len(history)+1)
	input = append(input, openai.Message{
		Role:    "user",
		Content: stablePrefix,
	})
	input = append(input, history...)
	return input
}

func appendWorkingContext(history []openai.Message, packet context.Packet) []openai.Message {
	return append(history, openai.Message{
		Role:    "user",
		Content: packet.RecentWorkingContext,
	})
}

func appendDecisionExchange(history []openai.Message, result openai.StreamResult, observation string) []openai.Message {
	return append(history, decisionResultHistoryMessage(result), observationHistoryMessage(observation))
}

func decisionResultHistoryMessage(result openai.StreamResult) openai.Message {
	lines := []string{"Function call returned by model:"}
	if len(result.FunctionCalls) == 0 {
		if strings.TrimSpace(result.OutputText) == "" {
			lines = append(lines, "none")
		} else {
			lines = append(lines, "output_text="+result.OutputText)
		}
		return openai.Message{Role: "assistant", Content: strings.Join(lines, "\n")}
	}
	for _, item := range result.FunctionCalls {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strings.TrimSpace(item.CallName)
		}
		if name == "" {
			name = "unknown"
		}
		lines = append(lines, "type="+item.Type)
		lines = append(lines, "name="+name)
		if strings.TrimSpace(item.CallID) != "" {
			lines = append(lines, "call_id="+item.CallID)
		}
		if strings.TrimSpace(item.ID) != "" {
			lines = append(lines, "item_id="+item.ID)
		}
		lines = append(lines, "arguments="+item.Arguments)
	}
	if strings.TrimSpace(result.OutputText) != "" {
		lines = append(lines, "output_text="+result.OutputText)
	}
	return openai.Message{Role: "assistant", Content: strings.Join(lines, "\n")}
}

func observationHistoryMessage(observation string) openai.Message {
	return openai.Message{
		Role:    "user",
		Content: "Observation result:\n" + observation,
	}
}
