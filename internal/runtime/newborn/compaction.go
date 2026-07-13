package newborn

import (
	"fmt"
	"strings"
	"time"

	"ai-arena/internal/openai"
)

const compactionMaxOutputTokens = 900

func (r *Runner) compactHistory(profile ResidentProfile, stablePrefix string, history *runHistory, state loopState, trigger CompactionTriggerReason, triggerDetail string, keepRecentRounds int, completedRounds int, verbose bool) CompactionEvent {
	started := time.Now().UTC()
	event := CompactionEvent{
		CompactionID:  fmt.Sprintf("compact-%s-%s", profile.Name, started.Format("20060102T150405.000000000Z")),
		RunID:         state.RunGroupID,
		Resident:      profile.Name,
		OccurredAt:    started.Format(time.RFC3339),
		TriggerReason: trigger,
		TriggerDetail: triggerDetail,
	}
	beforeInput := history.input(stablePrefix)
	event.TokensBefore = estimatePromptTokens(beforeInput)

	roundsToAbsorb := history.recentRounds - keepRecentRounds
	if roundsToAbsorb <= 0 && event.TokensBefore > modelContextTriggerTokens(profile.Model) {
		roundsToAbsorb = maxInt(1, history.recentRounds/4)
	}
	if roundsToAbsorb <= 0 {
		event.Outcome = CompactionOutcomeSilentTrim
		event.TokensAfter = event.TokensBefore
		event.DurationMs = int(time.Since(started).Milliseconds())
		return event
	}
	if roundsToAbsorb > history.recentRounds {
		roundsToAbsorb = history.recentRounds
	}

	segment := history.compactionSegment(roundsToAbsorb)
	input := buildCompactionPromptInput(stablePrefix, segment, state.LastSleepDepth)
	result, err := r.postStream(openai.RequestPayload{
		Model:           profile.Model,
		Instructions:    "只写便条正文，保持第一人称、自然、具体。不要解释规则。",
		PromptCacheKey:  fmt.Sprintf("arena-inner-continuity-%s-v1", profile.Name),
		Input:           input,
		MaxOutputTokens: compactionMaxOutputTokens,
		Stream:          true,
		Store:           false,
	}, verbose)
	event.DurationMs = int(time.Since(started).Milliseconds())
	if err != nil {
		event.Outcome = CompactionOutcomeFailed
		dropped := history.silentTrimRecentRounds(keepRecentRounds)
		event.RoundsAbsorbed = dropped
		event.TokensAfter = estimatePromptTokens(history.input(stablePrefix))
		return event
	}
	event.CachePrefixHitOnCompactionCall = result.CachedTokens > 0
	text := strings.TrimSpace(result.OutputText)
	if violation := checkCompactionEpistemicGuard(text); violation != nil {
		event.Outcome = CompactionOutcomeGuardRejected
		event.GuardRejectedSample = violation.Sample
		dropped := history.silentTrimRecentRounds(keepRecentRounds)
		event.RoundsAbsorbed = dropped
		event.TokensAfter = estimatePromptTokens(history.input(stablePrefix))
		return event
	}

	absorbedStart := completedRounds - history.recentRounds + 1
	if absorbedStart < 1 {
		absorbedStart = 1
	}
	absorbedEnd := absorbedStart + roundsToAbsorb - 1
	history.installSummaryPane(text, started, roundsToAbsorb, absorbedStart, absorbedEnd)
	event.Outcome = CompactionOutcomeSummarized
	event.RoundsAbsorbed = roundsToAbsorb
	if history.summaryPane != nil {
		event.SummaryPaneTokensAfter = history.summaryPane.ApproxTokens
	}
	event.TokensAfter = estimatePromptTokens(history.input(stablePrefix))
	return event
}

func modelContextTriggerTokens(model string) int {
	switch strings.TrimSpace(model) {
	case "gpt-5.5", "gpt-5.4":
		return 187500
	default:
		return 96000
	}
}
