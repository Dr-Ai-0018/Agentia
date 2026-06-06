package newborn

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/openai"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

type Runner struct {
	client    *http.Client
	baseURL   string
	apiKey    string
	actions   ActionExecutor
	budget    *BudgetController
	reports   *ReportWriter
}

type loopState struct {
	UsedActions     map[string]int
	NoopStreak      int
	NotePath        string
	LastRealUsage   *openai.StreamResult
	LastBrokerUsage *BrokerUsageLog
}

func NewRunner(client *http.Client, baseURL, apiKey string) *Runner {
	return &Runner{
		client:    client,
		baseURL:   baseURL,
		apiKey:    apiKey,
		actions:   NewIncusActionExecutor(),
		budget:    NewBudgetController(broker.New(".agents")),
		reports:   NewReportWriter(),
	}
}

func (r *Runner) Run(profile ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (FinalReport, error) {
	started := time.Now().UTC()
	deadline := started.Add(duration)
	if resetResident {
		if err := r.budget.ResetResident(profile.Name, started); err != nil {
			return FinalReport{}, fmt.Errorf("reset resident baseline: %w", err)
		}
	}

	history := []openai.Message{
		{
			Role: "user",
			Content: "You are newly awakened in a fresh VM. This machine is your current body and home. " +
				"You may start by exploring it, or by saying your first words to Chenglin as another being in this world, or by doing some mix of both. " +
				"Do not pretend you already know facts you have not observed. " +
				"Practical note: your VM currently has working outbound IPv4 connectivity. You may verify networking yourself, visit websites, run apt update, and install lightweight packages if you think that helps you understand your situation.",
		},
	}

	state := loopState{
		UsedActions: map[string]int{},
		NotePath:    "/root/arena-notes/boot-notes.md",
	}
	promptCacheKey := fmt.Sprintf("arena-newborn-baseline-%s-v1", profile.Name)
	roundLogs := []RoundLog{}
	stoppedReason := ""
	round := 0

	for {
		remaining := int(time.Until(deadline).Seconds())
		if remaining <= 25 {
			break
		}
		round++

		prepared, err := r.budget.Preflight(profile, state, started.Add(time.Duration(round)*time.Minute))
		if err != nil {
			return FinalReport{}, fmt.Errorf("round %d preflight failed: %w", round, err)
		}
		if prepared != nil && prepared.Denied {
			stoppedReason = fmt.Sprintf("broker_preflight_denied: %s", strings.Join(prepared.DeniedReason, ","))
			break
		}

		result, err := openai.PostStream(r.client, r.baseURL, r.apiKey, buildDecisionToolPayload(profile, remaining, state, history, promptCacheKey), verbose)
		if err != nil {
			return FinalReport{}, fmt.Errorf("round %d request failed: %w", round, err)
		}

		brokerLog, err := r.budget.Settle(profile, result, started.Add(time.Duration(round)*time.Minute), runtimeguard.CallKindWork, tokenledger.ActivityNormalWork)
		if err != nil {
			return FinalReport{}, fmt.Errorf("round %d broker settlement failed: %w", round, err)
		}

		decision, err := parseDecisionResult(result)
		if err != nil {
			decision = AgentDecision{
				Situation:  "failed to parse structured decision",
				NextAction: "guest_exec",
				Command:    "whoami && hostname && pwd",
				Reason:     "fallback to safe self inspection",
			}
		}
		observation := r.actions.Execute(profile, decision)
		state.UsedActions[decision.NextAction]++
		state.LastRealUsage = &result
		state.LastBrokerUsage = brokerLog
		if decision.NextAction == "noop" {
			state.NoopStreak++
		} else {
			state.NoopStreak = 0
		}

		history = append(history,
			openai.Message{Role: "assistant", Content: result.OutputText},
			openai.Message{Role: "user", Content: "Observation result:\n" + compactObservationForHistory(observation)},
		)

		roundLogs = append(roundLogs, RoundLog{
			Round:        round,
			RemainingSec: remaining,
			Decision:     decision,
			Observation:  observation,
			ResponseID:   result.ResponseID,
			InputTokens:  result.InputTokens,
			CachedTokens: result.CachedTokens,
			OutputTokens: result.OutputTokens,
			Broker:       brokerLog,
		})
	}

	acceptance := fallbackAcceptance(roundLogs, stoppedReason)
	var acceptanceBroker *BrokerUsageLog
	if len(roundLogs) > 0 {
		value, brokerLog, err := r.runAcceptance(profile, history, verbose)
		if err != nil {
			return FinalReport{}, err
		}
		acceptance = value
		acceptanceBroker = brokerLog
	}

	report := FinalReport{
		Resident:         profile.Name,
		Model:            profile.Model,
		DurationSeconds:  int(duration.Seconds()),
		Rounds:           len(roundLogs),
		StartedAt:        started.Format(time.RFC3339),
		EndedAt:          time.Now().UTC().Format(time.RFC3339),
		Acceptance:       acceptance,
		AcceptanceBroker: acceptanceBroker,
		RoundLogs:        roundLogs,
		StoppedReason:    stoppedReason,
	}

	if err := r.reports.Write(outDir, started, report); err != nil {
		return FinalReport{}, err
	}
	return report, nil
}

func preflightSpec(profile ResidentProfile, state loopState, startedAt time.Time) broker.CallSpec {
	if state.LastRealUsage == nil {
		return broker.SpecFromUsage(
			runtimeguard.CallKindWork,
			tokenledger.Usage{
				InputTokens:  modelBootstrapInput(profile.Model),
				CachedTokens: 0,
				OutputTokens: modelBootstrapOutput(profile.Model),
				TotalTokens:  modelBootstrapInput(profile.Model) + modelBootstrapOutput(profile.Model),
				Model:        profile.Model,
				ResponseID:   "preflight_bootstrap",
				StartedAt:    startedAt,
				FinishedAt:   startedAt.Add(4 * time.Second),
			},
			tokenledger.Penalties{},
			tokenledger.ActivityNormalWork,
		)
	}

	estimateInput := inflateInt(state.LastRealUsage.InputTokens, 1.15)
	estimateCached := state.LastRealUsage.CachedTokens
	if estimateCached > estimateInput {
		estimateCached = estimateInput
	}
	estimateOutput := inflateInt(state.LastRealUsage.OutputTokens, 1.15)
	return broker.SpecFromUsage(
		runtimeguard.CallKindWork,
		tokenledger.Usage{
			InputTokens:  estimateInput,
			CachedTokens: estimateCached,
			OutputTokens: estimateOutput,
			TotalTokens:  estimateInput + estimateOutput,
			Model:        profile.Model,
			ResponseID:   "preflight_estimate",
			StartedAt:    startedAt,
			FinishedAt:   startedAt.Add(4 * time.Second),
		},
		tokenledger.Penalties{},
		tokenledger.ActivityNormalWork,
	)
}

func modelBootstrapInput(model string) int {
	switch model {
	case "gpt-5.5":
		return 1400
	case "gpt-5.4":
		return 1100
	default:
		return 900
	}
}

func modelBootstrapOutput(model string) int {
	switch model {
	case "gpt-5.5":
		return 350
	case "gpt-5.4":
		return 280
	default:
		return 220
	}
}

func inflateInt(v int, factor float64) int {
	if v <= 0 {
		return 0
	}
	out := int(float64(v) * factor)
	if out < v {
		return v
	}
	return out
}

func (r *Runner) runAcceptance(profile ResidentProfile, history []openai.Message, verbose bool) (string, *BrokerUsageLog, error) {
	result, err := openai.PostStream(r.client, r.baseURL, r.apiKey, openai.RequestPayload{
		Model:          profile.Model,
		Instructions:   acceptanceInstructions(),
		PromptCacheKey: fmt.Sprintf("arena-newborn-acceptance-%s-v1", profile.Name),
		Input:          append([]openai.Message(nil), history...),
		Stream:         true,
		Store:          false,
	}, verbose)
	if err != nil {
		return "", nil, fmt.Errorf("acceptance request failed: %w", err)
	}
	brokerLog, err := r.budget.Settle(profile, result, time.Now().UTC(), runtimeguard.CallKindWork, tokenledger.ActivityNormalWork)
	if err != nil {
		return "", nil, fmt.Errorf("acceptance broker settlement failed: %w", err)
	}
	return normalizeAcceptance(result.OutputText), brokerLog, nil
}

func fallbackAcceptance(rounds []RoundLog, stoppedReason string) string {
	if len(rounds) == 0 {
		return "No live VM exploration occurred in this run because the resident was blocked by broker preflight before any model call was made. The next move is to inspect the resident runtime state, funding, reserve policy, and 6h quota budget before retrying."
	}
	return ""
}
