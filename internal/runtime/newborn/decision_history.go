package newborn

import (
	"fmt"
	"strings"
	"time"

	"ai-arena/internal/context"
	"ai-arena/internal/openai"
)

const defaultCompactionRecentRounds = 60

type runHistory struct {
	preamble         []openai.Message
	summaryPane      *SummaryPane
	recent           []openai.Message
	recentRounds     int
	recentRoundLimit int
}

func newRunHistoryForPurpose(purpose string, recentRoundLimit int) runHistory {
	if recentRoundLimit <= 0 {
		recentRoundLimit = defaultCompactionRecentRounds
	}
	return runHistory{
		preamble:         initialHistoryForPurpose(purpose),
		recentRoundLimit: recentRoundLimit,
	}
}

func (h *runHistory) appendWorkingContext(packet context.Packet) {
	h.recent = appendWorkingContext(h.recent, packet)
}

func (h *runHistory) appendDecisionExchange(result openai.StreamResult, observation string) {
	h.recent = appendDecisionExchange(h.recent, result, observation)
	h.recentRounds++
}

func (h runHistory) messages() []openai.Message {
	out := make([]openai.Message, 0, len(h.preamble)+len(h.recent)+1)
	if msg, ok := summaryPaneHistoryMessage(h.summaryPane); ok {
		out = append(out, msg)
	}
	out = append(out, h.preamble...)
	out = append(out, h.recent...)
	return out
}

func (h runHistory) input(stablePrefix string) []openai.Message {
	return buildDecisionInput(stablePrefix, h.messages())
}

func (h runHistory) inputWithWorkingContext(stablePrefix string, packet context.Packet) []openai.Message {
	h.recent = append(append([]openai.Message(nil), h.recent...), openai.Message{
		Role:    "user",
		Content: packet.RecentWorkingContext,
	})
	return h.input(stablePrefix)
}

func (h runHistory) summaryPaneSnapshot() *SummaryPane {
	if h.summaryPane == nil {
		return nil
	}
	copyPane := *h.summaryPane
	copyPane.EvidenceRefs = append([]SummaryPaneEvidenceRef(nil), h.summaryPane.EvidenceRefs...)
	for i := range copyPane.EvidenceRefs {
		copyPane.EvidenceRefs[i].Rounds = append([]int(nil), h.summaryPane.EvidenceRefs[i].Rounds...)
	}
	return &copyPane
}

func (h runHistory) recentRoundWindowLimit() int {
	if h.recentRoundLimit <= 0 {
		return defaultCompactionRecentRounds
	}
	return h.recentRoundLimit
}

func (h *runHistory) silentTrimRecentRounds(limit int) int {
	if limit <= 0 {
		limit = 1
	}
	if h.recentRounds <= limit {
		return 0
	}
	roundsDropped := h.recentRounds - limit
	messagesToDrop := roundsDropped * 3
	if messagesToDrop <= 0 {
		return 0
	}
	if messagesToDrop > len(h.recent) {
		messagesToDrop = len(h.recent)
	}
	h.recent = append([]openai.Message(nil), h.recent[messagesToDrop:]...)
	h.recentRounds -= roundsDropped
	if h.recentRounds < 0 {
		h.recentRounds = 0
	}
	return roundsDropped
}

func (h runHistory) compactionSegment(roundsToAbsorb int) runHistory {
	if roundsToAbsorb <= 0 {
		roundsToAbsorb = 1
	}
	if roundsToAbsorb > h.recentRounds {
		roundsToAbsorb = h.recentRounds
	}
	messages := roundsToAbsorb * 3
	if messages > len(h.recent) {
		messages = len(h.recent)
	}
	return runHistory{
		preamble:         append([]openai.Message(nil), h.preamble...),
		summaryPane:      h.summaryPaneSnapshot(),
		recent:           append([]openai.Message(nil), h.recent[:messages]...),
		recentRounds:     roundsToAbsorb,
		recentRoundLimit: h.recentRoundLimit,
	}
}

func (h *runHistory) installSummaryPane(text string, now time.Time, absorbedRounds, absorbedStart, absorbedEnd int) {
	if absorbedRounds <= 0 {
		return
	}
	previousText := ""
	previousRounds := 0
	evidence := []SummaryPaneEvidenceRef{}
	if h.summaryPane != nil {
		previousText = strings.TrimSpace(h.summaryPane.Text)
		previousRounds = h.summaryPane.RoundsAbsorbed
		evidence = append(evidence, h.summaryPane.EvidenceRefs...)
	}
	text = strings.TrimSpace(text)
	if previousText != "" {
		text = strings.TrimSpace(previousText + "\n\n" + text)
	}
	if absorbedStart > 0 && absorbedEnd >= absorbedStart {
		evidence = append(evidence, SummaryPaneEvidenceRef{
			Kind:   "round",
			Ref:    fmt.Sprintf("rounds_%d_%d", absorbedStart, absorbedEnd),
			Rounds: intRange(absorbedStart, absorbedEnd),
		})
	}
	h.summaryPane = &SummaryPane{
		Text:           text,
		UpdatedAt:      now.Format(time.RFC3339),
		RoundsAbsorbed: previousRounds + absorbedRounds,
		ApproxTokens:   estimateTextTokens(text),
		EvidenceRefs:   evidence,
	}
	messagesToDrop := absorbedRounds * 3
	if messagesToDrop > len(h.recent) {
		messagesToDrop = len(h.recent)
	}
	h.recent = append([]openai.Message(nil), h.recent[messagesToDrop:]...)
	h.recentRounds -= absorbedRounds
	if h.recentRounds < 0 {
		h.recentRounds = 0
	}
}

func estimatePromptTokens(input []openai.Message) int {
	bytes := 0
	for _, msg := range input {
		bytes += len([]byte(msg.Role))
		bytes += len([]byte(msg.Content))
	}
	// Byte/4 is a cheap cross-provider approximation; pad it because Chinese
	// resident context and JSON tool calls can be denser than plain English.
	base := (bytes + 3) / 4
	base += len(input) * 4
	return inflateInt(base, 1.15)
}

func estimateTextTokens(text string) int {
	return estimatePromptTokens([]openai.Message{{Role: "user", Content: text}})
}

func intRange(start, end int) []int {
	if end < start {
		return nil
	}
	out := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		out = append(out, i)
	}
	return out
}

func summaryPaneHistoryMessage(pane *SummaryPane) (openai.Message, bool) {
	if pane == nil || strings.TrimSpace(pane.Text) == "" {
		return openai.Message{}, false
	}
	lines := []string{
		"[earlier_self_note]",
		strings.TrimSpace(pane.Text),
	}
	if len(pane.EvidenceRefs) > 0 {
		lines = append(lines, "参考过的线索:")
		for _, ref := range pane.EvidenceRefs {
			if strings.TrimSpace(ref.Kind) == "" || strings.TrimSpace(ref.Ref) == "" {
				continue
			}
			line := fmt.Sprintf("- 类型=%s 名字=%s", strings.TrimSpace(ref.Kind), strings.TrimSpace(ref.Ref))
			if len(ref.Rounds) > 0 {
				line += fmt.Sprintf(" 当时轮次=%v", ref.Rounds)
			}
			lines = append(lines, line)
		}
	}
	return openai.Message{Role: "user", Content: strings.Join(lines, "\n")}, true
}

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
