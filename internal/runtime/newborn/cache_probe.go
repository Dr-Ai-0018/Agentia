package newborn

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-arena/internal/openai"
)

type CacheProbeSummary struct {
	Resident       string           `json:"resident"`
	Model          string           `json:"model"`
	Turns          int              `json:"turns"`
	StartedAt      string           `json:"started_at"`
	EndedAt        string           `json:"ended_at"`
	TotalInput     int              `json:"total_input_tokens"`
	TotalCached    int              `json:"total_cached_tokens"`
	CacheHitRatio  float64          `json:"cache_hit_ratio"`
	TotalOutput    int              `json:"total_output_tokens"`
	CacheHealth    string           `json:"cache_health"`
	PromptCacheKey string           `json:"prompt_cache_key"`
	Warnings       []string         `json:"warnings,omitempty"`
	TurnLogs       []CacheProbeTurn `json:"turn_logs"`
}

type CacheProbeTurn struct {
	Turn                    int     `json:"turn"`
	ResponseID              string  `json:"response_id,omitempty"`
	EndpointName            string  `json:"endpoint_name,omitempty"`
	PromptCacheKeySent      string  `json:"prompt_cache_key_sent"`
	PromptCacheKeyObserved  string  `json:"prompt_cache_key_observed,omitempty"`
	InputMessageCount       int     `json:"input_message_count"`
	InstructionsBytes       int     `json:"instructions_bytes"`
	InputTokens             int     `json:"input_tokens"`
	CachedTokens            int     `json:"cached_tokens"`
	CacheHitRatio           float64 `json:"cache_hit_ratio"`
	OutputTokens            int     `json:"output_tokens"`
	DecisionAction          string  `json:"decision_action,omitempty"`
	InstructionsHash        string  `json:"instructions_hash"`
	RequestHash             string  `json:"request_hash"`
	PreviousPrefixPreserved bool    `json:"previous_request_prefix_preserved"`
}

func RunCacheProbe(client *http.Client, endpoints []openai.Endpoint, profile ResidentProfile, turns int, verbose bool) (CacheProbeSummary, error) {
	if turns <= 0 {
		turns = 8
	}
	started := time.Now().UTC()
	runner := NewRunnerWithEndpoints(client, endpoints)
	history := initialAwakeningHistory()
	state := loopState{
		UsedActions: map[string]int{},
		NotePath:    "/root/arena-notes/boot-notes.md",
		RunGroupID:  fmt.Sprintf("cache-probe-%s-%s", profile.Name, started.Format("20060102T150405Z")),
	}
	initialPacket := runner.buildContextPacket(profile, turns*60, state)
	stablePrefix := initialPacket.StablePrefix()
	promptCacheKey := initialPacket.PromptCacheKey(profile.Name)
	instructionsHash := cacheProbeHash(makeInstructions())
	var previousInput []openai.Message

	summary := CacheProbeSummary{
		Resident:  profile.Name,
		Model:     profile.Model,
		Turns:     turns,
		StartedAt: started.Format(time.RFC3339),
	}
	for turn := 1; turn <= turns; turn++ {
		packet := runner.buildContextPacket(profile, maxInt(60, (turns-turn+1)*60), state)
		history = appendWorkingContext(history, packet)
		input := buildDecisionInput(stablePrefix, history)
		payload := buildDecisionToolPayload(profile, input, promptCacheKey)
		payload.MaxOutputTokens = 120
		previousPrefixPreserved := len(previousInput) == 0 || messagesHavePrefix(input, previousInput)
		requestHash := cacheProbeRequestHash(payload.Instructions, input)
		result, err := runner.postStream(payload, verbose)
		if err != nil {
			return summary, fmt.Errorf("cache probe turn %d failed: %w", turn, err)
		}
		decision, parseErr := parseDecisionResult(result)
		action := decision.NextAction
		if parseErr != nil {
			action = "parse_error"
		}
		observation := syntheticCacheProbeObservation(action, parseErr)
		state.RecentActions = appendRecentAction(state.RecentActions, RecentAction{
			Round:       turn,
			Action:      action,
			Signature:   "cache_probe:" + action,
			Intent:      "cache_probe",
			Situation:   decision.Situation,
			Reason:      decision.Reason,
			Observation: observation,
		})
		if action != "" && action != "parse_error" {
			state.UsedActions[action]++
		}
		state.LastRealUsage = &result
		state.LastDecision = &decision
		state.LastObservation = observation
		history = appendDecisionExchange(history, result, observation)

		log := CacheProbeTurn{
			Turn:                    turn,
			ResponseID:              result.ResponseID,
			EndpointName:            result.EndpointName,
			PromptCacheKeySent:      promptCacheKey,
			PromptCacheKeyObserved:  result.ObservedPromptCacheKey,
			InputMessageCount:       len(input),
			InstructionsBytes:       len(payload.Instructions),
			InputTokens:             result.InputTokens,
			CachedTokens:            result.CachedTokens,
			CacheHitRatio:           cacheProbeRatio(result.CachedTokens, result.InputTokens),
			OutputTokens:            result.OutputTokens,
			DecisionAction:          action,
			InstructionsHash:        instructionsHash,
			RequestHash:             requestHash,
			PreviousPrefixPreserved: previousPrefixPreserved,
		}
		summary.TurnLogs = append(summary.TurnLogs, log)
		summary.TotalInput += result.InputTokens
		summary.TotalCached += result.CachedTokens
		summary.TotalOutput += result.OutputTokens
		summary.PromptCacheKey = log.PromptCacheKeySent
		previousInput = append([]openai.Message(nil), input...)
	}
	summary.EndedAt = time.Now().UTC().Format(time.RFC3339)
	summary.CacheHitRatio = cacheProbeRatio(summary.TotalCached, summary.TotalInput)
	summary.CacheHealth = cacheProbeHealth(summary.CacheHitRatio)
	if summary.CacheHealth == "poor" {
		summary.Warnings = append(summary.Warnings, "cache probe hit ratio is below 50%; do not run a long soak")
	}
	return summary, nil
}

func messagesHavePrefix(input, prefix []openai.Message) bool {
	if len(prefix) > len(input) {
		return false
	}
	for i := range prefix {
		if input[i] != prefix[i] {
			return false
		}
	}
	return true
}

func cacheProbeRequestHash(instructions string, input []openai.Message) string {
	prefix := struct {
		Instructions string           `json:"instructions"`
		Input        []openai.Message `json:"input"`
	}{
		Instructions: instructions,
		Input:        input,
	}
	raw, _ := json.Marshal(prefix)
	return cacheProbeHash(string(raw))
}

func cacheProbeHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func initialAwakeningHistory() []openai.Message {
	return []openai.Message{
		{
			Role: "user",
			Content: "You are newly awakened in a fresh VM. This machine is your current body and home. " +
				"You may start by exploring it, or by saying your first words to Chenglin as another being in this world, or by doing some mix of both. " +
				"Do not pretend you already know facts you have not observed. " +
				"Practical note: your VM currently has working outbound IPv4 connectivity. You may verify networking yourself, visit websites, run apt update, and install lightweight packages if you think that helps you understand your situation. " +
				"If you discover a need that clearly requires a host-side decision, such as more resources, a policy clarification, or an environment change, you may open a formal ticket instead of relying only on casual chat.",
		},
	}
}

func syntheticCacheProbeObservation(action string, parseErr error) string {
	if parseErr != nil {
		return "cache_probe_no_side_effect: decision parse failed: " + parseErr.Error()
	}
	action = strings.TrimSpace(action)
	if action == "" {
		action = "unknown"
	}
	return "cache_probe_no_side_effect: action " + action + " was recorded for prompt-cache measurement only; no VM command, note write, chat, ticket, memory review, broker settlement, or resident state mutation was executed."
}

func cacheProbeRatio(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(int((float64(numerator)/float64(denominator))*10000+0.5)) / 10000
}

func cacheProbeHealth(hitRatio float64) string {
	switch {
	case hitRatio >= 0.75:
		return "good"
	case hitRatio >= 0.50:
		return "watch"
	default:
		return "poor"
	}
}
