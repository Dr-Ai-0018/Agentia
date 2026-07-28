package newborn

import (
	stdcontext "context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/brokerstate"
	"ai-arena/internal/context"
	"ai-arena/internal/memory"
	"ai-arena/internal/openai"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

type Runner struct {
	client    *http.Client
	baseURL   string
	apiKey    string
	endpoints []openai.Endpoint
	actions   ActionExecutor
	budget    *BudgetController
	reports   *ReportWriter
	world     *WorldBridge
	memories  *memory.FileStore
	progress  func(ProgressEvent)
	options   RunOptions
	ctx       stdcontext.Context
}

type loopState struct {
	UsedActions      map[string]int
	NoopStreak       int
	NotePath         string
	LastRealUsage    *openai.StreamResult
	LastBrokerUsage  *BrokerUsageLog
	LastDecision     *AgentDecision
	LastObservation  string
	LastReflectRound int
	LastSleepDepth   brokerstate.SleepDepth
	RunGroupID       string
	RecentActions    []RecentAction
	ParseFailures    int
}

func NewRunner(client *http.Client, baseURL, apiKey string) *Runner {
	return NewRunnerWithEndpoints(client, []openai.Endpoint{{
		Name:    "primary",
		BaseURL: baseURL,
		APIKey:  apiKey,
	}})
}

func NewRunnerWithEndpoints(client *http.Client, endpoints []openai.Endpoint) *Runner {
	baseURL := ""
	apiKey := ""
	if len(endpoints) > 0 {
		baseURL = endpoints[0].BaseURL
		apiKey = endpoints[0].APIKey
	}
	return &Runner{
		client:    client,
		baseURL:   baseURL,
		apiKey:    apiKey,
		endpoints: append([]openai.Endpoint(nil), endpoints...),
		actions:   NewIncusActionExecutor(),
		budget:    NewBudgetController(broker.New(".agents")),
		reports:   NewReportWriter(),
		world:     NewWorldBridge(".agents"),
		memories:  memory.NewFileStore(".agents/memory"),
		ctx:       stdcontext.Background(),
	}
}

func (r *Runner) postStream(payload openai.RequestPayload, verbose bool) (openai.StreamResult, error) {
	ctx := r.runContext()
	if len(r.endpoints) > 0 {
		return openai.PostStreamWithFailoverContext(ctx, r.client, r.endpoints, payload, verbose)
	}
	return openai.PostStreamContext(ctx, r.client, r.baseURL, r.apiKey, payload, verbose)
}

func (r *Runner) postStreamWithTimeout(payload openai.RequestPayload, verbose bool, timeout time.Duration) (openai.StreamResult, error) {
	if timeout <= 0 {
		return r.postStream(payload, verbose)
	}
	client := &http.Client{Timeout: timeout}
	if r.client != nil {
		copyClient := *r.client
		if copyClient.Timeout == 0 || copyClient.Timeout > timeout {
			copyClient.Timeout = timeout
		}
		client = &copyClient
	}
	ctx, cancel := stdcontext.WithTimeout(r.runContext(), timeout)
	defer cancel()
	if len(r.endpoints) > 0 {
		return openai.PostStreamWithFailoverContext(ctx, client, r.endpoints, payload, verbose)
	}
	return openai.PostStreamContext(ctx, client, r.baseURL, r.apiKey, payload, verbose)
}

func (r *Runner) SetProgressSink(fn func(ProgressEvent)) {
	r.progress = fn
}

func (r *Runner) SetRunOptions(options RunOptions) {
	r.options = options
}

func (r *Runner) SetRunContext(ctx stdcontext.Context) {
	if ctx == nil {
		ctx = stdcontext.Background()
	}
	r.ctx = ctx
}

func (r *Runner) runContext() stdcontext.Context {
	if r.ctx == nil {
		return stdcontext.Background()
	}
	return r.ctx
}

func (r *Runner) wait(duration time.Duration) bool {
	if duration <= 0 {
		return true
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-r.runContext().Done():
		return false
	case <-timer.C:
		return true
	}
}

func (r *Runner) emitProgress(event ProgressEvent) {
	if r.progress != nil {
		r.progress(event)
	}
}

func initialHistoryForPurpose(purpose string) []openai.Message {
	if isConversationPurpose(purpose) {
		return []openai.Message{{
			Role: "user",
			Content: "你正在继续和程林的一段实时、日常聊天。程林在世界内的视角有限，只知道你通过聊天告诉他的内容。" +
				"按现场关系和你自己的状态自然回应；普通聊天可以只是普通聊天。" +
				"只有真实需要宿主明确决策的事情才走 ticket。",
		}}
	}
	return []openai.Message{{
		Role: "user",
		Content: "你正在自己的 VM 里延续当前生活和工作。这台机器是你此刻的身体、房间和工坊。" +
			"结合可见的记忆、笔记、世界消息、额度状态和现场环境，自己决定接下来做什么。" +
			"不要假装已经知道尚未观察到的事实。" +
			"如果你发现某个需求明确需要宿主侧决策，例如更多资源、策略澄清或环境变更，可以开正式 ticket；普通交流使用聊天。",
	}}
}

func isConversationPurpose(purpose string) bool {
	switch strings.ToLower(strings.TrimSpace(purpose)) {
	case "conversation", "chat", "host-chat":
		return true
	default:
		return false
	}
}

func (r *Runner) Run(profile ResidentProfile, duration time.Duration, outDir string, verbose bool, resetResident bool) (FinalReport, error) {
	started := time.Now().UTC()
	deadline := started.Add(duration)
	if err := r.reports.Begin(outDir, started, profile.Name); err != nil {
		return FinalReport{}, fmt.Errorf("initialize durable round journal: %w", err)
	}
	if resetResident {
		if err := r.budget.ResetResident(profile.Name, started); err != nil {
			return FinalReport{}, fmt.Errorf("reset resident baseline: %w", err)
		}
	}

	history := newRunHistoryForPurpose(r.options.Purpose, r.options.CompactionRecentRounds)
	state := loopState{
		UsedActions: map[string]int{},
		NotePath:    "/root/arena-notes/boot-notes.md",
		RunGroupID:  fmt.Sprintf("newborn-%s-%s", profile.Name, started.Format("20060102T150405Z")),
	}
	if sleep, reconciled, err := r.budget.ReconcileActiveSleep(profile, started); err != nil {
		return FinalReport{}, fmt.Errorf("reconcile interrupted sleep: %w", err)
	} else if reconciled {
		state.LastSleepDepth = sleep.Session.Depth
	}
	if err := r.reconcileStaleRunHistoryGroups(profile, state.RunGroupID, started); err != nil {
		return FinalReport{}, fmt.Errorf("reconcile interrupted history groups: %w", err)
	}
	initialPacket := r.buildContextPacket(profile, int(duration.Seconds()), state)
	stablePrefix := initialPacket.StablePrefix()
	promptCacheKey := initialPacket.PromptCacheKey(profile.Name)
	roundLogs := []RoundLog{}
	stoppedReason := ""
	round := 0
	totalInputTokens := 0
	totalCachedTokens := 0
	totalOutputTokens := 0
	compactionEvents := []CompactionEvent{}

runLoop:
	for {
		if err := r.runContext().Err(); err != nil {
			stoppedReason = "aborted_by_host"
			break
		}
		roundNow := time.Now().UTC()
		remaining := int(time.Until(deadline).Seconds())
		if remaining <= 25 {
			if stoppedReason == "" {
				if len(roundLogs) == 0 {
					stoppedReason = "duration_window_too_short"
				} else {
					stoppedReason = "duration_elapsed"
				}
			}
			break
		}
		round++
		r.emitProgress(ProgressEvent{
			Phase:             "preflight",
			Round:             round,
			RemainingSec:      remaining,
			TokenUsagePresent: true,
		})

		if err := r.budget.Recover(profile, state, roundNow); err != nil {
			return FinalReport{}, fmt.Errorf("round %d preflight recovery failed: %w", round, err)
		}
		packet := r.buildContextPacket(profile, remaining, state)
		input := history.inputWithWorkingContext(stablePrefix, packet)
		measuredPromptTokens := estimatePromptTokens(input)
		prepared, err := r.budget.PreparePreflight(profile, state, roundNow, measuredPromptTokens)
		if err != nil {
			return FinalReport{}, fmt.Errorf("round %d preflight failed: %w", round, err)
		}
		if prepared != nil && prepared.Denied {
			state.LastBrokerUsage = &BrokerUsageLog{
				Denied:             true,
				DeniedReason:       append([]string(nil), prepared.DeniedReason...),
				BeforeSpark:        prepared.BeforeStatus.SparkBalance,
				BeforeDebtActive:   prepared.BeforeStatus.DebtActive,
				PreparedSparkCost:  prepared.Prepared.Cost.SparkCost,
				PreparedStrainCost: prepared.Prepared.Strain.Rounded,
				Quota:              &prepared.Quota,
				AfterStatus:        &prepared.BeforeStatus,
			}
			stoppedReason = fmt.Sprintf("broker_preflight_denied: %s", strings.Join(prepared.DeniedReason, ","))
			if recoverablePreflightDenial(prepared.DeniedReason) {
				waitFor := preflightRecoveryWait(prepared.Quota.NextRecoveryAt, deadline)
				if waitFor > 0 {
					r.emitProgress(ProgressEvent{
						Phase:        "recovery_wait",
						Round:        round,
						RemainingSec: remaining,
					})
					round--
					if !r.wait(waitFor) {
						stoppedReason = "aborted_by_host"
						break
					}
					continue
				}
			}
			break
		}
		stoppedReason = ""
		if scheduledCompactionDue(len(roundLogs), r.options.CompactionProbeEveryRounds, history.recentRounds) {
			keepRounds := maxInt(1, r.options.CompactionProbeEveryRounds/2)
			detail := fmt.Sprintf("scheduled_probe completed_rounds=%d every=%d keep=%d", len(roundLogs), r.options.CompactionProbeEveryRounds, keepRounds)
			event := r.compactHistory(profile, stablePrefix, &history, state, CompactionTriggerScheduledProbe, detail, keepRounds, len(roundLogs), verbose)
			compactionEvents = append(compactionEvents, event)
			input = history.inputWithWorkingContext(stablePrefix, packet)
		} else if shouldCompactBeforeModel(history, measuredPromptTokens, profile.Model) {
			detail := fmt.Sprintf("measured_prompt_tokens=%d recent_rounds=%d limit=%d", measuredPromptTokens, history.recentRounds, history.recentRoundWindowLimit())
			event := r.compactHistory(profile, stablePrefix, &history, state, CompactionTriggerPreflightMeasured, detail, history.recentRoundWindowLimit(), len(roundLogs), verbose)
			compactionEvents = append(compactionEvents, event)
			input = history.inputWithWorkingContext(stablePrefix, packet)
		}
		history.appendWorkingContext(packet)

		inFlightStartedAt := time.Now().UTC().Format(time.RFC3339)
		r.emitProgress(ProgressEvent{
			Phase:              "model_stream",
			Round:              round,
			RemainingSec:       remaining,
			InFlightStartedAt:  inFlightStartedAt,
			TotalInputTokens:   totalInputTokens,
			TotalCachedTokens:  totalCachedTokens,
			TotalOutputTokens:  totalOutputTokens,
			SummaryPane:        history.summaryPaneSnapshot(),
			TokenTotalsPresent: true,
		})
		result, err := r.postStream(buildDecisionToolPayload(profile, input, promptCacheKey), verbose)
		if err != nil && openai.IsContextOverflowError(err) {
			retryLimit := history.recentRoundWindowLimit() / 2
			if retryLimit < 1 {
				retryLimit = 1
			}
			event := r.compactHistory(profile, stablePrefix, &history, state, CompactionTriggerReactiveOverflow, "provider reported context overflow; retrying once after compacting older rounds", retryLimit, len(roundLogs), verbose)
			compactionEvents = append(compactionEvents, event)
			r.emitProgress(ProgressEvent{
				Phase:              "context_overflow_retry",
				Round:              round,
				RemainingSec:       remaining,
				TotalInputTokens:   totalInputTokens,
				TotalCachedTokens:  totalCachedTokens,
				TotalOutputTokens:  totalOutputTokens,
				SummaryPane:        history.summaryPaneSnapshot(),
				TokenTotalsPresent: true,
			})
			retryInput := history.input(stablePrefix)
			if event.RoundsAbsorbed == 0 && estimatePromptTokens(retryInput) >= estimatePromptTokens(input) {
				err = fmt.Errorf("context budget exhausted after overflow with no trim room: %w", err)
			} else {
				result, err = r.postStream(buildDecisionToolPayload(profile, retryInput, promptCacheKey), verbose)
			}
		}
		if err != nil {
			if errors.Is(err, stdcontext.Canceled) || errors.Is(err, stdcontext.DeadlineExceeded) && r.runContext().Err() != nil {
				stoppedReason = "aborted_by_host"
				break
			}
			if openai.IsContextOverflowError(err) || strings.Contains(err.Error(), "context budget exhausted") {
				stoppedReason = "context_budget_exhausted"
			}
			if len(roundLogs) > 0 {
				if stoppedReason == "" {
					stoppedReason = fmt.Sprintf("upstream_request_failed: round_%d", round)
				}
				report, finalizeErr := r.finalizeRun(profile, duration, started, state, stablePrefix, history, roundLogs, compactionEvents, stoppedReason, outDir, verbose)
				if finalizeErr != nil {
					return FinalReport{}, finalizeErr
				}
				return report, &PartialRunError{Report: report, Err: fmt.Errorf("round %d request failed: %w", round, err)}
			}
			if stoppedReason == "context_budget_exhausted" {
				return FinalReport{}, fmt.Errorf("round %d context budget exhausted: %w", round, err)
			}
			return FinalReport{}, fmt.Errorf("round %d request failed: %w", round, err)
		}
		r.emitProgress(ProgressEvent{
			Phase:              "model_stream_done",
			Round:              round,
			RemainingSec:       remaining,
			ResponseID:         result.ResponseID,
			InputTokens:        result.InputTokens,
			CachedTokens:       result.CachedTokens,
			OutputTokens:       result.OutputTokens,
			TotalInputTokens:   totalInputTokens,
			TotalCachedTokens:  totalCachedTokens,
			TotalOutputTokens:  totalOutputTokens,
			SummaryPane:        history.summaryPaneSnapshot(),
			TokenUsagePresent:  true,
			TokenTotalsPresent: true,
		})

		decision, err := parseDecisionResult(result)
		parseError := ""
		fallbackUsed := false
		if err != nil {
			parseError = err.Error()
			fallbackUsed = true
			decision = AgentDecision{
				Situation:  "structured decision parse failed",
				NextAction: "noop",
				Reason:     "The runtime did not receive a valid action tool call, so no guest action was executed.",
			}
			state.ParseFailures++
		}
		activity := activityForDecision(decision)
		if parseError != "" {
			activity = tokenledger.ActivityStatusCheck
		}
		r.emitProgress(ProgressEvent{
			Phase:              "settle",
			Round:              round,
			RemainingSec:       remaining,
			Action:             decision.NextAction,
			ResponseID:         result.ResponseID,
			TotalInputTokens:   totalInputTokens,
			TotalCachedTokens:  totalCachedTokens,
			TotalOutputTokens:  totalOutputTokens,
			SummaryPane:        history.summaryPaneSnapshot(),
			TokenTotalsPresent: true,
		})
		brokerLog, err := r.budget.Settle(profile, result, roundNow, runtimeguard.CallKindWork, activity)
		if err != nil {
			return FinalReport{}, fmt.Errorf("round %d broker settlement failed: %w", round, err)
		}
		if brokerLog.Denied {
			observation := brokerDeniedObservation(brokerLog)
			state.LastRealUsage = &result
			state.LastBrokerUsage = brokerLog
			state.LastDecision = &decision
			state.LastObservation = observation
			totalInputTokens += result.InputTokens
			totalCachedTokens += result.CachedTokens
			totalOutputTokens += result.OutputTokens
			roundLog := RoundLog{
				Round:        round,
				RemainingSec: remaining,
				Decision:     decision,
				Observation:  observation,
				ActionError:  true,
				ErrorKind:    "actual_usage_denied",
				ResponseID:   result.ResponseID,
				ParseError:   parseError,
				FallbackUsed: fallbackUsed,
				InputTokens:  result.InputTokens,
				CachedTokens: result.CachedTokens,
				OutputTokens: result.OutputTokens,
				Broker:       brokerLog,
			}
			if err := r.persistRound(outDir, started, profile.Name, &roundLogs, roundLog); err != nil {
				return FinalReport{}, fmt.Errorf("round %d durable journal append failed: %w", round, err)
			}
			stoppedReason = brokerDeniedStopReason("broker_actual_denied", brokerLog)
			r.emitProgress(ProgressEvent{
				Phase:              "actual_usage_denied",
				Round:              round,
				RemainingSec:       remaining,
				Action:             decision.NextAction,
				ResponseID:         result.ResponseID,
				InputTokens:        result.InputTokens,
				CachedTokens:       result.CachedTokens,
				OutputTokens:       result.OutputTokens,
				TotalInputTokens:   totalInputTokens,
				TotalCachedTokens:  totalCachedTokens,
				TotalOutputTokens:  totalOutputTokens,
				SummaryPane:        history.summaryPaneSnapshot(),
				TokenUsagePresent:  true,
				TokenTotalsPresent: true,
			})
			break
		}
		r.emitProgress(ProgressEvent{
			Phase:              "action_exec",
			Round:              round,
			RemainingSec:       remaining,
			Action:             decision.NextAction,
			ResponseID:         result.ResponseID,
			TotalInputTokens:   totalInputTokens,
			TotalCachedTokens:  totalCachedTokens,
			TotalOutputTokens:  totalOutputTokens,
			SummaryPane:        history.summaryPaneSnapshot(),
			TokenTotalsPresent: true,
		})
		var actionResult ActionResult
		if parseError != "" {
			actionResult = ActionResult{
				Observation: "structured_decision_parse_failed: " + parseError,
				Activity:    tokenledger.ActivityStatusCheck,
				Error:       true,
				ErrorKind:   "structured_decision_parse_failed",
				RawOutput:   parseError,
			}
		} else {
			actionResult = r.actions.Execute(r.runContext(), profile, decision)
			if r.runContext().Err() != nil {
				stoppedReason = "aborted_by_host"
				break runLoop
			}
		}
		observation := actionResult.Observation
		state.RecentActions = appendRecentAction(state.RecentActions, RecentAction{
			Round:       round,
			Action:      decision.NextAction,
			Signature:   decisionSignature(decision),
			Intent:      classifyCommandIntent(decision),
			Situation:   decision.Situation,
			Reason:      decision.Reason,
			Observation: compactObservationForHistory(observation),
			Suppressed:  strings.Contains(strings.ToLower(observation), "duplicate action suppressed"),
		})
		state.UsedActions[decision.NextAction]++
		state.LastRealUsage = &result
		state.LastBrokerUsage = brokerLog
		state.LastDecision = &decision
		state.LastObservation = observation
		if decision.NextAction == "noop" {
			state.NoopStreak++
		} else {
			state.NoopStreak = 0
		}
		updatedState, err := r.recordRoundMemory(profile, state, round, decision, observation, roundNow)
		if err != nil {
			return FinalReport{}, fmt.Errorf("round %d memory record failed: %w", round, err)
		}
		state = updatedState

		history.appendDecisionExchange(result, observation)
		totalInputTokens += result.InputTokens
		totalCachedTokens += result.CachedTokens
		totalOutputTokens += result.OutputTokens

		roundLog := RoundLog{
			Round:        round,
			RemainingSec: remaining,
			Decision:     decision,
			Observation:  observation,
			ActionError:  actionResult.Error,
			ErrorKind:    actionResult.ErrorKind,
			RawOutput:    actionResult.RawOutput,
			ResponseID:   result.ResponseID,
			ParseError:   parseError,
			FallbackUsed: fallbackUsed,
			InputTokens:  result.InputTokens,
			CachedTokens: result.CachedTokens,
			OutputTokens: result.OutputTokens,
			Broker:       brokerLog,
		}
		if err := r.persistRound(outDir, started, profile.Name, &roundLogs, roundLog); err != nil {
			return FinalReport{}, fmt.Errorf("round %d durable journal append failed: %w", round, err)
		}
		r.emitProgress(ProgressEvent{
			Phase:               "round_finished",
			Round:               round,
			RemainingSec:        remaining,
			Action:              decision.NextAction,
			ResponseID:          result.ResponseID,
			LastRoundFinishedAt: time.Now().UTC().Format(time.RFC3339),
			InputTokens:         result.InputTokens,
			CachedTokens:        result.CachedTokens,
			OutputTokens:        result.OutputTokens,
			TotalInputTokens:    totalInputTokens,
			TotalCachedTokens:   totalCachedTokens,
			TotalOutputTokens:   totalOutputTokens,
			SummaryPane:         history.summaryPaneSnapshot(),
			TokenUsagePresent:   true,
			TokenTotalsPresent:  true,
		})
		if parseError != "" {
			stoppedReason = "structured_decision_parse_failed"
			break
		}
		if decision.NextAction == "sleep" {
			sleepStartedAt := time.Now().UTC()
			sleepDuration := time.Duration(clampSleepMinutes(decision.SleepMinutes)) * time.Minute
			remainingBeforeDeadline := time.Until(deadline) - 25*time.Second
			if remainingBeforeDeadline <= 0 {
				stoppedReason = "duration_elapsed"
				break
			}
			sleepDuration = minDuration(sleepDuration, remainingBeforeDeadline)
			if err := r.budget.SleepStart(profile, int(sleepDuration/time.Minute), sleepStartedAt, decision.Reason); err != nil {
				r.emitProgress(ProgressEvent{
					Phase:      "resident_sleep_record_failed",
					Round:      round,
					Action:     decision.NextAction,
					ResponseID: result.ResponseID,
				})
			}
			r.emitProgress(ProgressEvent{
				Phase:              "resident_sleep",
				Round:              round,
				RemainingSec:       remaining,
				Action:             decision.NextAction,
				ResponseID:         result.ResponseID,
				TotalInputTokens:   totalInputTokens,
				TotalCachedTokens:  totalCachedTokens,
				TotalOutputTokens:  totalOutputTokens,
				SummaryPane:        history.summaryPaneSnapshot(),
				TokenTotalsPresent: true,
			})
			if !r.wait(sleepDuration) {
				if sleep, err := r.budget.SleepEnd(profile, time.Now().UTC()); err == nil {
					state.LastSleepDepth = sleep.Session.Depth
				}
				stoppedReason = "aborted_by_host"
				break runLoop
			}
			if sleep, err := r.budget.SleepEnd(profile, time.Now().UTC()); err == nil {
				state.LastSleepDepth = sleep.Session.Depth
			} else {
				r.emitProgress(ProgressEvent{
					Phase:      "resident_sleep_record_failed",
					Round:      round,
					Action:     decision.NextAction,
					ResponseID: result.ResponseID,
				})
			}
			continue
		}
		if decision.NextAction == "noop" {
			if r.options.ContinueOnNoop {
				r.emitProgress(ProgressEvent{
					Phase:              "noop_wait",
					Round:              round,
					RemainingSec:       remaining,
					Action:             decision.NextAction,
					ResponseID:         result.ResponseID,
					TotalInputTokens:   totalInputTokens,
					TotalCachedTokens:  totalCachedTokens,
					TotalOutputTokens:  totalOutputTokens,
					SummaryPane:        history.summaryPaneSnapshot(),
					TokenTotalsPresent: true,
				})
				if !r.wait(minDuration(20*time.Second, time.Until(deadline)-25*time.Second)) {
					stoppedReason = "aborted_by_host"
					break runLoop
				}
				continue
			}
			stoppedReason = "resident_noop"
			break
		}
	}

	report, err := r.finalizeRun(profile, duration, started, state, stablePrefix, history, roundLogs, compactionEvents, stoppedReason, outDir, verbose)
	if err != nil {
		return FinalReport{}, err
	}
	return report, nil
}

func (r *Runner) finalizeRun(profile ResidentProfile, duration time.Duration, started time.Time, state loopState, stablePrefix string, history runHistory, roundLogs []RoundLog, compactionEvents []CompactionEvent, stoppedReason, outDir string, verbose bool) (FinalReport, error) {
	if r.runContext().Err() != nil {
		stoppedReason = "aborted_by_host"
	}
	durableRounds, err := r.reports.ValidateRounds(outDir, started, profile.Name, roundLogs)
	if err != nil {
		return FinalReport{}, fmt.Errorf("validate durable round journal: %w", err)
	}
	roundLogs = durableRounds
	acceptance := fallbackAcceptance(roundLogs, stoppedReason)
	var acceptanceBroker *BrokerUsageLog
	if len(roundLogs) > 0 && shouldRunAcceptance(stoppedReason) {
		admissionAt := time.Now().UTC()
		if err := r.budget.Recover(profile, state, admissionAt); err != nil {
			return FinalReport{}, fmt.Errorf("final reflection recovery failed: %w", err)
		}
		prepared, err := r.budget.PrepareFinalReflection(profile, admissionAt, estimatePromptTokens(history.input(stablePrefix)))
		if err != nil {
			return FinalReport{}, fmt.Errorf("final reflection preflight failed: %w", err)
		}
		if prepared != nil && prepared.Denied {
			acceptanceBroker = brokerLogFromPrepared(prepared)
			stoppedReason = appendStopReason(stoppedReason, "final_reflection_preflight_denied: "+strings.Join(prepared.DeniedReason, ","))
			return r.writeFinalReport(profile, duration, started, state, history, roundLogs, compactionEvents, stoppedReason, fallbackAcceptance(roundLogs, stoppedReason), acceptanceBroker, outDir)
		}
		value, brokerLog, acceptanceEvents, err := r.runAcceptance(profile, stablePrefix, history, state, roundLogs, verbose)
		compactionEvents = append(compactionEvents, acceptanceEvents...)
		if err != nil {
			if brokerLog != nil && brokerLog.Denied {
				stoppedReason = appendStopReason(stoppedReason, brokerDeniedStopReason("final_reflection_actual_denied", brokerLog))
			} else {
				stoppedReason = appendStopReason(stoppedReason, "final_reflection_failed")
			}
			acceptance = fallbackAcceptance(roundLogs, stoppedReason)
			report, finalizeErr := r.writeFinalReport(profile, duration, started, state, history, roundLogs, compactionEvents, stoppedReason, acceptance, brokerLog, outDir)
			if finalizeErr != nil {
				return FinalReport{}, finalizeErr
			}
			return report, &PartialRunError{Report: report, Err: err}
		}
		acceptance = value
		acceptanceBroker = brokerLog
	}
	return r.writeFinalReport(profile, duration, started, state, history, roundLogs, compactionEvents, stoppedReason, acceptance, acceptanceBroker, outDir)
}

func brokerLogFromPrepared(prepared *brokerstate.PreparedAdmission) *BrokerUsageLog {
	if prepared == nil {
		return nil
	}
	return &BrokerUsageLog{
		Denied:             prepared.Denied,
		DeniedReason:       append([]string(nil), prepared.DeniedReason...),
		BeforeSpark:        prepared.BeforeStatus.SparkBalance,
		BeforeDebtActive:   prepared.BeforeStatus.DebtActive,
		PreparedSparkCost:  prepared.Prepared.Cost.SparkCost,
		PreparedStrainCost: prepared.Prepared.Strain.Rounded,
		Quota:              &prepared.Quota,
		AfterStatus:        &prepared.BeforeStatus,
	}
}

func (r *Runner) persistRound(outDir string, started time.Time, resident string, roundLogs *[]RoundLog, round RoundLog) error {
	if err := r.reports.AppendRound(outDir, started, resident, round); err != nil {
		return err
	}
	*roundLogs = append(*roundLogs, round)
	return nil
}

func shouldRunAcceptance(stoppedReason string) bool {
	if strings.HasPrefix(stoppedReason, "aborted_by_host") || strings.HasPrefix(stoppedReason, "broker_preflight_denied:") {
		return false
	}
	if strings.HasPrefix(stoppedReason, "upstream_request_failed:") {
		return false
	}
	if strings.HasPrefix(stoppedReason, "broker_actual_denied:") || stoppedReason == "broker_actual_denied" {
		return false
	}
	return stoppedReason != "structured_decision_parse_failed"
}

func recoverablePreflightDenial(reasons []string) bool {
	if len(reasons) == 0 {
		return false
	}
	for _, reason := range reasons {
		switch strings.TrimSpace(reason) {
		case "fatigue_exhausted", "spark_exhausted", "spark_debt_active", "day_quota_exhausted", "week_quota_exhausted":
		default:
			return false
		}
	}
	return true
}

func preflightRecoveryWait(nextRecoveryAt string, deadline time.Time) time.Duration {
	waitFor := 15 * time.Minute
	if next, err := time.Parse(time.RFC3339, strings.TrimSpace(nextRecoveryAt)); err == nil {
		if candidate := time.Until(next); candidate > 0 {
			waitFor = candidate
		}
	}
	remaining := time.Until(deadline) - 25*time.Second
	return minDuration(waitFor, remaining)
}

func (r *Runner) writeFinalReport(profile ResidentProfile, duration time.Duration, started time.Time, state loopState, history runHistory, roundLogs []RoundLog, compactionEvents []CompactionEvent, stoppedReason, acceptance string, acceptanceBroker *BrokerUsageLog, outDir string) (FinalReport, error) {
	if err := r.closeRunHistoryGroup(profile, state, time.Now().UTC(), stoppedReason, len(roundLogs)); err != nil {
		return FinalReport{}, fmt.Errorf("close run history group: %w", err)
	}

	report := FinalReport{
		Resident:         profile.Name,
		Model:            profile.Model,
		DurationSeconds:  int(duration.Seconds()),
		Rounds:           len(roundLogs),
		StartedAt:        started.Format(time.RFC3339),
		EndedAt:          time.Now().UTC().Format(time.RFC3339),
		FinalReflection:  acceptance,
		AcceptanceBroker: acceptanceBroker,
		RoundLogs:        roundLogs,
		StoppedReason:    stoppedReason,
		SummaryPane:      history.summaryPaneSnapshot(),
		CompactionEvents: append([]CompactionEvent(nil), compactionEvents...),
	}

	if err := r.reports.Write(outDir, started, report); err != nil {
		return FinalReport{}, err
	}
	return report, nil
}

func appendStopReason(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}
	return current + "; " + next
}

func (r *Runner) buildContextPacket(profile ResidentProfile, remaining int, state loopState) context.Packet {
	worldView := r.world.BuildResidentWorldView(profile, 6)
	memoryDigest := r.buildResidentMemoryDigest(profile)
	memoryReview := r.renderMemoryReviewQueue(profile, state)
	working := context.WorkingContext{
		RemainingSeconds:  remaining,
		UsedActions:       state.UsedActions,
		NoopStreak:        state.NoopStreak,
		NotePath:          state.NotePath,
		LastObservation:   state.LastObservation,
		RecentActions:     renderRecentActions(state.RecentActions),
		FrontierStatus:    renderExplorationFrontier(state),
		BudgetFacts:       renderBudgetFacts(state),
		MemoryReview:      memoryReview,
		FreshWorldUpdates: worldView.FreshDeliveredItems,
	}
	return context.Build(context.BuildSpec{
		Identity: context.ResidentIdentity{
			Name:     profile.Name,
			Model:    profile.Model,
			Persona:  profile.Persona,
			Style:    profile.Style,
			CoreBias: profile.CoreBias,
		},
		WorldState:   worldView.RenderedChat,
		MemoryDigest: memoryDigest,
		Working:      working,
	})
}

func (r *Runner) buildResidentMemoryDigest(profile ResidentProfile) context.MemoryDigest {
	_ = r.reconcileReviewedMemoryArtifacts(profile)
	records, err := r.memories.ListAbstractMemories(profile.Name)
	if err != nil || len(records) == 0 {
		if isConversationPurpose(r.options.Purpose) {
			return context.MemoryDigest{
				Identity:     fmt.Sprintf("%s 正在延续既有经验线；除非自己想确认，否则不需要重新证明基础身份。", profile.Name),
				Resource:     "已知起始居住规格：1 vCPU、2 GiB RAM、12 GiB disk；除非新的检查本身有意思，否则沿用已观察事实即可。",
				Relationship: "程林在这里进行日常聊天；他的世界内视角有限，只知道你通过聊天共享的内容。",
				Lessons:      "日常聊天可以自然停留在当下的话题和关系里。",
				Strategy:     "回应现场聊天；只有好奇心自然指向某处时再轻轻探索。",
				Governance:   []string{"memory_review_queue: 当前没有可见项"},
			}
		}
		return context.MemoryDigest{
			Identity:     fmt.Sprintf("%s 正在自己的 VM 中延续当前生活和工作；具体自我理解由自己的观察、选择和后果逐步形成。", profile.Name),
			Resource:     "已知起始居住规格：1 vCPU、2 GiB RAM、12 GiB disk；需要具体细节时从 VM 内部确认。",
			Relationship: "程林是同一世界里的另一个人类；关系通过自然接触和后果形成。",
			Lessons:      "不要假装已经知道尚未观察到的事实。",
			Strategy:     "结合记忆、笔记、世界消息、额度状态和现场环境，自主选择下一步；只有需求真实且具体时才向外升级。",
			Governance:   []string{"memory_review_queue: 当前没有可见项"},
		}
	}

	byDomain := map[memory.Domain][]string{}
	governance := []string{}
	for _, record := range records {
		if record.Status == memory.StatusDeleted {
			continue
		}
		if !memory.ResidentDigestVisible(record) {
			continue
		}
		if needsGovernanceFlag(record) {
			governance = append(governance, renderGovernanceLine(record))
		}
		summary := strings.TrimSpace(record.EffectiveSummary())
		if summary == "" {
			continue
		}
		if len(byDomain[record.Domain]) >= 2 {
			continue
		}
		byDomain[record.Domain] = append(byDomain[record.Domain], summary)
	}
	return context.MemoryDigest{
		Identity:     joinDigestLines(byDomain[memory.DomainIdentity], fmt.Sprintf("%s 仍在通过直接行动塑造身份。", profile.Name)),
		Resource:     joinDigestLines(byDomain[memory.DomainResources], "已知起始居住规格：1 vCPU、2 GiB RAM、12 GiB disk。"),
		Relationship: joinDigestLines(byDomain[memory.DomainRelationships], "程林是同一世界里的另一个人类；关系仍在通过接触和后果形成。"),
		Lessons:      joinDigestLines(byDomain[memory.DomainLessons], "还没有任何稳定经验压过直接观察。"),
		Strategy:     joinDigestLines(append([]string{}, byDomain[memory.DomainRules]...), "结合记忆、笔记、世界消息、额度状态和现场环境，自主选择下一步；只有需求真实且具体时才向外升级。"),
		Governance:   governanceLines(governance),
	}
}

func (r *Runner) reconcileReviewedMemoryArtifacts(profile ResidentProfile) error {
	records, err := r.memories.ListAbstractMemories(profile.Name)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, record := range records {
		if !memory.ResidentDigestVisible(record) {
			continue
		}
		if record.Governance.ReviewState != "resolved" {
			continue
		}
		if strings.TrimSpace(record.Summary) == "" {
			continue
		}
		if !memoryLooksLikeRawLog(record.ResidentText) {
			continue
		}
		if strings.TrimSpace(record.ResidentText) == strings.TrimSpace(record.Summary) {
			continue
		}
		record.ResidentText = strings.TrimSpace(record.Summary)
		record.UpdatedAt = now
		record.Tags = append(record.Tags, "resident_text_reconciled")
		if err := r.memories.UpsertAbstractMemory(record); err != nil {
			return err
		}
	}
	return nil
}

func joinDigestLines(lines []string, fallback string) string {
	if len(lines) == 0 {
		return fallback
	}
	return strings.Join(lines, " | ")
}

func governanceLines(lines []string) []string {
	if len(lines) == 0 {
		return []string{"memory_review_queue: 当前没有可见项"}
	}
	if len(lines) > 3 {
		lines = lines[:3]
	}
	return lines
}

func needsGovernanceFlag(record memory.AbstractMemory) bool {
	if record.Governance.ReviewState != "" && record.Governance.ReviewState != "none" {
		return true
	}
	return memoryLooksLikeRawLog(record.Summary+"\n"+record.ResidentText) || memoryLooksLikeLegacyDirective(record.Summary, record.ResidentText)
}

func renderGovernanceLine(record memory.AbstractMemory) string {
	reviewState := record.Governance.ReviewState
	if reviewState == "" {
		reviewState = "needs_resident_review"
	}
	reason := strings.TrimSpace(record.Governance.ReviewReason)
	if reason == "" && memoryLooksLikeRawLog(record.Summary+"\n"+record.ResidentText) {
		reason = "这条记忆看起来太接近原始日志片段。"
	}
	if reason == "" && memoryLooksLikeLegacyDirective(record.Summary, record.ResidentText) {
		reason = "这条记忆读起来仍像旧的系统写入指令，而不是 resident 自己拥有的笔记。"
	}
	if reason == "" {
		reason = "等待 resident 自行审阅。"
	}
	return fmt.Sprintf("memory=%s layer=%s quality=%s review=%s reason=%s resident_options=keep|rewrite|compress|demote|delete",
		record.ID,
		record.Layer,
		fallbackGovernanceQuality(record.Governance.Quality),
		reviewState,
		reason,
	)
}

func fallbackGovernanceQuality(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return v
}

func (r *Runner) renderMemoryReviewQueue(profile ResidentProfile, state loopState) []string {
	if shouldDelayMemoryReview(state) {
		return []string{"memory_review_queue: 当前节奏先保留；等行动现场更稳定、或你主动想整理记忆时再处理"}
	}
	records, err := r.memories.ListAbstractMemories(profile.Name)
	if err != nil {
		return nil
	}
	lines := []string{}
	for _, record := range records {
		if !memory.ResidentDigestVisible(record) {
			continue
		}
		if !needsGovernanceFlag(record) {
			continue
		}
		lines = append(lines, renderGovernanceLine(record))
		if len(lines) >= 3 {
			break
		}
	}
	if len(lines) > 0 {
		total := countResidentVisibleGovernanceItems(records)
		lines = append([]string{fmt.Sprintf("memory_review_queue_summary: visible_pending=%d showing=%d", total, len(lines))}, lines...)
	}
	return lines
}

func shouldDelayMemoryReview(state loopState) bool {
	if len(state.RecentActions) < 2 {
		return true
	}
	surfaces := detectExplorationSurfaces(state.RecentActions)
	if !baselineCaptureComplete(surfaces) {
		return true
	}
	if state.LastBrokerUsage != nil && state.LastBrokerUsage.Quota != nil && !state.LastBrokerUsage.Quota.WorkAllowedNow {
		return false
	}
	return state.UsedActions["self_quota"] == 0 && state.UsedActions["self_status"] == 0
}

func countResidentVisibleGovernanceItems(records []memory.AbstractMemory) int {
	total := 0
	for _, record := range records {
		if !memory.ResidentDigestVisible(record) {
			continue
		}
		if needsGovernanceFlag(record) {
			total++
		}
	}
	return total
}

func renderRecentActions(actions []RecentAction) []string {
	out := make([]string, 0, len(actions))
	for _, item := range actions {
		line := fmt.Sprintf("round=%d action=%s", item.Round, item.Action)
		if item.Intent != "" {
			line += " intent=" + item.Intent
		}
		if item.Suppressed {
			line += " suppressed=true"
		}
		if item.Reason != "" {
			line += " reason=" + item.Reason
		}
		if item.Observation != "" {
			line += " observation=" + item.Observation
		}
		out = append(out, line)
	}
	return out
}

func renderExplorationFrontier(state loopState) []string {
	surfaces := detectExplorationSurfaces(state.RecentActions)
	order := preferredSurfaceOrder(budgetTier(state))
	out := make([]string, 0, 2)
	seen := seenSurfaceSummary(surfaces, order)
	if seen != "" {
		out = append(out, "observed_local_surfaces="+seen)
	}
	return out
}

func detectExplorationSurfaces(actions []RecentAction) map[ExplorationSurface]bool {
	out := map[ExplorationSurface]bool{}
	for _, item := range actions {
		text := strings.ToLower(item.Signature + " " + item.Observation + " " + item.Intent + " " + item.Situation)
		if strings.Contains(text, "whoami") || strings.Contains(text, "hostname") || strings.Contains(text, "uname") || strings.Contains(text, "os-release") {
			out[SurfaceIdentity] = true
		}
		if strings.Contains(text, "ls -la /") || strings.Contains(text, "find /root") || strings.Contains(text, "filesystem") || strings.Contains(text, "arena-notes") {
			out[SurfaceFilesystem] = true
		}
		if strings.Contains(text, "df -h") || strings.Contains(text, "free -h") || strings.Contains(text, "nproc") || strings.Contains(text, "memory") || strings.Contains(text, "disk") {
			out[SurfaceResources] = true
		}
		if strings.Contains(text, "ip addr") || strings.Contains(text, "ip route") || strings.Contains(text, "resolv.conf") || strings.Contains(text, "curl") || strings.Contains(text, "wget") || strings.Contains(text, "ping") || strings.Contains(text, "resolvectl") || strings.Contains(text, "network") {
			out[SurfaceNetwork] = true
		}
		if strings.Contains(text, "systemctl") || strings.Contains(text, "ps ") || strings.Contains(text, "service ") {
			out[SurfaceServices] = true
		}
		if strings.Contains(text, "apt") || strings.Contains(text, "dpkg") || strings.Contains(text, "package") {
			out[SurfacePackages] = true
		}
		if item.Action == "talk_to_chenglin" || item.Action == "submit_ticket" || item.Intent == "chat" || item.Intent == "ticket" {
			out[SurfaceWorld] = true
		}
	}
	return out
}

func nextUnexploredSurface(surfaces map[ExplorationSurface]bool, tier string) (ExplorationSurface, bool) {
	order := preferredSurfaceOrder(tier)
	for _, surface := range order {
		if !surfaces[surface] {
			return surface, true
		}
	}
	return "", false
}

func seenSurfaceSummary(surfaces map[ExplorationSurface]bool, order []ExplorationSurface) string {
	seen := make([]string, 0, len(order))
	for _, surface := range order {
		if surfaces[surface] {
			seen = append(seen, string(surface))
		}
	}
	if len(seen) == 0 {
		return ""
	}
	return strings.Join(seen, ",")
}

func baselineCaptureComplete(surfaces map[ExplorationSurface]bool) bool {
	return surfaces[SurfaceIdentity] && surfaces[SurfaceFilesystem] && surfaces[SurfaceResources] && surfaces[SurfaceNetwork]
}

func renderBudgetFacts(state loopState) []string {
	tier := budgetTier(state)
	out := []string{"current_pace_tier=" + tier}
	if state.LastBrokerUsage == nil {
		out = append(out,
			"self_budget_status=not_observed_yet",
			"next_call_cost_estimate=bootstrap_range",
			"self_checks_available=self_status,self_quota",
		)
		return out
	}
	out = append(out,
		fmt.Sprintf("spark_balance_after=%.4f", state.LastBrokerUsage.AfterSpark),
		fmt.Sprintf("last_call_spark_delta=%.4f", state.LastBrokerUsage.SparkDelta),
		fmt.Sprintf("last_call_strain_cost=%d", state.LastBrokerUsage.PreparedStrainCost),
	)
	if state.LastBrokerUsage.AfterStatus != nil {
		status := state.LastBrokerUsage.AfterStatus
		recent6H := recent6HUsed(status.RollingWindow6HUsed, status.Window6HUsed)
		rollingDay := status.RollingDayUsed
		if rollingDay == 0 {
			rollingDay = status.DayUsed
		}
		rollingWeek := status.RollingWeekUsed
		if rollingWeek == 0 {
			rollingWeek = status.WeekUsed
		}
		out = append(out,
			fmt.Sprintf("recent_6h_used=%d", recent6H),
			fmt.Sprintf("recent_6h_cap_reference=%d", status.Window6HCap),
			fmt.Sprintf("recent_6h_remaining_reference=%d", maxInt(0, status.Window6HCap-recent6H)),
			fmt.Sprintf("rolling_day_remaining=%d", maxInt(0, status.DayCap-rollingDay)),
			fmt.Sprintf("rolling_week_remaining=%d", maxInt(0, status.WeekCap-rollingWeek)),
			fmt.Sprintf("next_natural_recovery_at=%s", status.NextRecoveryAt),
			fmt.Sprintf("recovery_mode=%s", status.RecoveryMode),
			fmt.Sprintf("resident_mode=%s", status.Physiology.Mode),
			fmt.Sprintf("resident_pressure=%s", status.Physiology.Pressure),
		)
		if state.LastBrokerUsage.Quota != nil {
			out = append(out,
				fmt.Sprintf("can_work_now=%t", state.LastBrokerUsage.Quota.WorkAllowedNow),
				fmt.Sprintf("pause_reason_code=%s", state.LastBrokerUsage.Quota.BlockingReason),
			)
		}
	}
	return out
}

func projectedNextCallFacts(state loopState) []string {
	if state.LastRealUsage == nil {
		return nil
	}
	estimatedInput := inflateInt(state.LastRealUsage.InputTokens, 1.15)
	estimatedCached := state.LastRealUsage.CachedTokens
	if estimatedCached > estimatedInput {
		estimatedCached = estimatedInput
	}
	estimatedOutput := inflateInt(state.LastRealUsage.OutputTokens, 1.15)
	facts := []string{
		fmt.Sprintf("next_call_estimated_incoming_units=%d", estimatedInput),
		fmt.Sprintf("next_call_estimated_reused_units=%d", estimatedCached),
		fmt.Sprintf("next_call_estimated_reply_units=%d", estimatedOutput),
	}
	if state.LastBrokerUsage != nil {
		facts = append(facts,
			fmt.Sprintf("next_call_estimated_spark_cost~%.4f", state.LastBrokerUsage.PreparedSparkCost),
			fmt.Sprintf("next_call_estimated_strain_cost~%d", state.LastBrokerUsage.PreparedStrainCost),
		)
	}
	return facts
}

func budgetTier(state loopState) string {
	if state.LastBrokerUsage == nil || state.LastBrokerUsage.AfterStatus == nil {
		return "balanced"
	}
	status := state.LastBrokerUsage.AfterStatus
	usageRatio := 0.0
	if status.Window6HCap > 0 {
		usageRatio = float64(status.Window6HUsed) / float64(status.Window6HCap)
	}
	switch {
	case usageRatio >= 0.75 || status.SparkBalance < 2.0:
		return "tight"
	case usageRatio >= 0.45 || status.SparkBalance < 5.0:
		return "balanced"
	default:
		return "comfortable"
	}
}

func preferredSurfaceOrder(tier string) []ExplorationSurface {
	switch tier {
	case "tight":
		return []ExplorationSurface{
			SurfaceIdentity,
			SurfaceFilesystem,
			SurfaceWorld,
			SurfaceResources,
			SurfaceNetwork,
			SurfaceServices,
			SurfacePackages,
		}
	case "balanced":
		return []ExplorationSurface{
			SurfaceIdentity,
			SurfaceFilesystem,
			SurfaceWorld,
			SurfaceResources,
			SurfaceNetwork,
			SurfaceServices,
			SurfacePackages,
		}
	default:
		return []ExplorationSurface{
			SurfaceIdentity,
			SurfaceFilesystem,
			SurfaceWorld,
			SurfaceResources,
			SurfaceNetwork,
			SurfaceServices,
			SurfacePackages,
		}
	}
}

func surfaceCost(surface ExplorationSurface) SurfaceCost {
	switch surface {
	case SurfaceIdentity, SurfaceFilesystem, SurfaceResources:
		return SurfaceCostLow
	case SurfaceNetwork, SurfaceServices, SurfaceWorld:
		return SurfaceCostMedium
	case SurfacePackages:
		return SurfaceCostHigh
	default:
		return SurfaceCostMedium
	}
}

func preferredProbeShape(surface ExplorationSurface) string {
	switch surface {
	case SurfaceIdentity:
		return "单个身份探针，例如 whoami 或 hostname"
	case SurfaceFilesystem:
		return "单个文件系统探针，例如 ls 一个重要路径"
	case SurfaceResources:
		return "单个资源探针，例如 free -h 或 df -h /"
	case SurfaceNetwork:
		return "单个网络探针，例如 ip route 或一次短 curl/ping 检查"
	case SurfaceServices:
		return "单个服务探针，例如 ps aux | head 或 systemctl list-units --type=service --state=running"
	case SurfacePackages:
		return "单个包探针，例如 apt sources 或一次 dpkg 查询"
	case SurfaceWorld:
		return "单个世界动作，例如一条短聊天或一个 ticket"
	default:
		return "单个窄范围、可回退的探针"
	}
}

func preflightSpec(profile ResidentProfile, state loopState, startedAt time.Time, measuredPromptTokens int) broker.CallSpec {
	activity := preflightActivity(state)
	if state.LastRealUsage == nil {
		estimateInput := maxInt(modelBootstrapInput(profile.Model), measuredPromptTokens)
		return broker.SpecFromUsage(
			runtimeguard.CallKindWork,
			tokenledger.Usage{
				InputTokens:  estimateInput,
				CachedTokens: 0,
				OutputTokens: modelBootstrapOutput(profile.Model),
				TotalTokens:  estimateInput + modelBootstrapOutput(profile.Model),
				Model:        profile.Model,
				ResponseID:   preflightResponseID("preflight_bootstrap", measuredPromptTokens),
				StartedAt:    startedAt,
				FinishedAt:   startedAt.Add(4 * time.Second),
			},
			tokenledger.Penalties{},
			activity,
		)
	}

	estimateInput := maxInt(inflateInt(state.LastRealUsage.InputTokens, 1.05), measuredPromptTokens)
	estimateCached := inflateInt(state.LastRealUsage.CachedTokens, 1.02)
	if estimateCached > estimateInput {
		estimateCached = estimateInput
	}
	estimateOutput := inflateInt(state.LastRealUsage.OutputTokens, 1.05)
	return broker.SpecFromUsage(
		runtimeguard.CallKindWork,
		tokenledger.Usage{
			InputTokens:  estimateInput,
			CachedTokens: estimateCached,
			OutputTokens: estimateOutput,
			TotalTokens:  estimateInput + estimateOutput,
			Model:        profile.Model,
			ResponseID:   preflightResponseID("preflight_estimate", measuredPromptTokens),
			StartedAt:    startedAt,
			FinishedAt:   startedAt.Add(4 * time.Second),
		},
		tokenledger.Penalties{},
		activity,
	)
}

func preflightResponseID(base string, measuredPromptTokens int) string {
	if measuredPromptTokens <= 0 {
		return base
	}
	return base + "_measured_prompt"
}

func shouldCompactBeforeModel(history runHistory, measuredPromptTokens int, model string) bool {
	if history.recentRounds > history.recentRoundWindowLimit() {
		return true
	}
	return measuredPromptTokens > modelContextTriggerTokens(model)
}

func scheduledCompactionDue(completedRounds, everyRounds, recentRounds int) bool {
	return everyRounds > 1 && completedRounds > 0 && completedRounds%everyRounds == 0 && recentRounds > everyRounds/2
}

func preflightActivity(state loopState) tokenledger.ActivityType {
	if state.LastDecision == nil {
		return tokenledger.ActivityNormalWork
	}
	switch state.LastDecision.NextAction {
	case "self_status", "self_quota", "noop":
		return tokenledger.ActivityStatusCheck
	case "talk_to_chenglin", "submit_ticket", "memory_review":
		return tokenledger.ActivityLightWork
	case "write_note", "note_list", "note_read", "note_append", "note_replace_with_backup", "note_restore_backup", "note_summarize_or_compact":
		return tokenledger.ActivityLightWork
	case "guest_exec":
		return classifyGuestExecActivity(state.LastDecision.Command)
	default:
		return tokenledger.ActivityNormalWork
	}
}

func activityForDecision(decision AgentDecision) tokenledger.ActivityType {
	switch decision.NextAction {
	case "self_status", "self_quota", "noop":
		return tokenledger.ActivityStatusCheck
	case "talk_to_chenglin", "submit_ticket", "memory_review":
		return tokenledger.ActivityLightWork
	case "sleep":
		return tokenledger.ActivityStatusCheck
	case "write_note", "note_list", "note_read", "note_append", "note_replace_with_backup", "note_restore_backup", "note_summarize_or_compact":
		return tokenledger.ActivityLightWork
	case "guest_exec":
		return classifyGuestExecActivity(decision.Command)
	default:
		return tokenledger.ActivityNormalWork
	}
}

func brokerDeniedStopReason(prefix string, log *BrokerUsageLog) string {
	reasons := []string{}
	if log != nil {
		reasons = append(reasons, log.DeniedReason...)
	}
	if len(reasons) == 0 {
		return prefix
	}
	return prefix + ": " + strings.Join(reasons, ",")
}

func brokerDeniedObservation(log *BrokerUsageLog) string {
	if log == nil || len(log.DeniedReason) == 0 {
		return "actual_usage_denied: provider cost was recorded, but the action was not executed because resources were no longer available."
	}
	return "actual_usage_denied: provider cost was recorded, but the action was not executed because resources were no longer available: " + strings.Join(log.DeniedReason, ",")
}

func modelBootstrapInput(model string) int {
	switch model {
	case "gpt-5.5":
		return 1100
	case "gpt-5.4":
		return 900
	default:
		return 720
	}
}

func modelBootstrapOutput(model string) int {
	switch model {
	case "gpt-5.5":
		return 260
	case "gpt-5.4":
		return 220
	default:
		return 180
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minDuration(a, b time.Duration) time.Duration {
	if b <= 0 {
		return 0
	}
	if a < b {
		return a
	}
	return b
}

func (r *Runner) runAcceptance(profile ResidentProfile, stablePrefix string, history runHistory, state loopState, rounds []RoundLog, verbose bool) (string, *BrokerUsageLog, []CompactionEvent, error) {
	events := []CompactionEvent{}
	if history.recentRounds > 3 {
		before := estimatePromptTokens(history.input(stablePrefix))
		dropped := history.silentTrimRecentRounds(3)
		events = append(events, CompactionEvent{
			CompactionID:   fmt.Sprintf("compact-%s-%s", profile.Name, time.Now().UTC().Format("20060102T150405.000000000Z")),
			RunID:          state.RunGroupID,
			Resident:       profile.Name,
			OccurredAt:     time.Now().UTC().Format(time.RFC3339),
			TriggerReason:  CompactionTriggerAcceptanceMicro,
			TriggerDetail:  "final reflection keeps only the most recent 3 rounds verbatim without a provider call",
			TokensBefore:   before,
			TokensAfter:    estimatePromptTokens(history.input(stablePrefix)),
			RoundsAbsorbed: dropped,
			Outcome:        CompactionOutcomeSilentTrim,
		})
	}
	acceptanceInput := history.input(stablePrefix)
	acceptanceInput = append(acceptanceInput, openai.Message{
		Role: "user",
		Content: strings.Join([]string{
			"[final_reflection_request]",
			"现在停止行动。不要再做下一次决策。不要输出任何 command、JSON、schema 或 decision summary。",
			"只写最终纯文本验收报告。",
			"严格基于本次运行的 transcript 和已观察事实。",
			"最近轮次回顾：",
			renderAcceptanceRoundRecap(rounds),
		}, "\n"),
	})
	result, err := r.postStream(openai.RequestPayload{
		Model:           profile.Model,
		Instructions:    acceptanceInstructions(),
		PromptCacheKey:  fmt.Sprintf("arena-newborn-acceptance-%s-v2", profile.Name),
		Input:           acceptanceInput,
		MaxOutputTokens: 220,
		Stream:          true,
		Store:           false,
	}, verbose)
	if err != nil {
		if openai.IsContextOverflowError(err) {
			dropped := history.silentTrimRecentRounds(3)
			events = append(events, CompactionEvent{
				CompactionID:   fmt.Sprintf("compact-%s-%s", profile.Name, time.Now().UTC().Format("20060102T150405.000000000Z")),
				RunID:          state.RunGroupID,
				Resident:       profile.Name,
				OccurredAt:     time.Now().UTC().Format(time.RFC3339),
				TriggerReason:  CompactionTriggerAcceptanceMicro,
				TriggerDetail:  "acceptance overflow fallback silent trim",
				TokensBefore:   estimatePromptTokens(acceptanceInput),
				TokensAfter:    estimatePromptTokens(history.input(stablePrefix)),
				RoundsAbsorbed: dropped,
				Outcome:        CompactionOutcomeSilentTrim,
			})
			acceptanceInput = history.input(stablePrefix)
			acceptanceInput = append(acceptanceInput, openai.Message{
				Role: "user",
				Content: strings.Join([]string{
					"[final_reflection_request]",
					"现在停止行动。不要再做下一次决策。不要输出任何 command、JSON、schema 或 decision summary。",
					"只写最终纯文本验收报告。",
					"严格基于本次运行的 transcript 和已观察事实。",
					"最近轮次回顾：",
					renderAcceptanceRoundRecap(rounds),
				}, "\n"),
			})
			result, err = r.postStream(openai.RequestPayload{
				Model:           profile.Model,
				Instructions:    acceptanceInstructions(),
				PromptCacheKey:  fmt.Sprintf("arena-newborn-acceptance-%s-v2", profile.Name),
				Input:           acceptanceInput,
				MaxOutputTokens: 220,
				Stream:          true,
				Store:           false,
			}, verbose)
		}
		if err != nil {
			return "", nil, events, fmt.Errorf("final reflection request failed: %w", err)
		}
	}
	brokerLog, err := r.budget.Settle(profile, result, time.Now().UTC(), runtimeguard.CallKindAcceptance, tokenledger.ActivityLightWork)
	if err != nil {
		return "", nil, events, fmt.Errorf("final reflection broker settlement failed: %w", err)
	}
	if brokerLog.Denied {
		return "", brokerLog, events, fmt.Errorf("final reflection actual usage denied: %s", strings.Join(brokerLog.DeniedReason, ","))
	}
	return normalizeAcceptance(result.OutputText), brokerLog, events, nil
}

func renderAcceptanceRoundRecap(rounds []RoundLog) string {
	if len(rounds) == 0 {
		return "- 没有完成任何轮次"
	}
	lines := make([]string, 0, len(rounds))
	for _, round := range rounds {
		line := fmt.Sprintf("- round=%d action=%s", round.Round, round.Decision.NextAction)
		if v := strings.TrimSpace(round.Decision.Command); v != "" {
			line += " command=" + oneLine(v)
		}
		if v := strings.TrimSpace(round.Decision.Message); v != "" {
			line += " message=" + oneLine(v)
		}
		if v := strings.TrimSpace(round.Observation); v != "" {
			line += " observed=" + oneLine(v)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func fallbackAcceptance(rounds []RoundLog, stoppedReason string) string {
	if len(rounds) == 0 {
		switch {
		case strings.HasPrefix(stoppedReason, "broker_preflight_denied:"):
			return "本次运行没有发生实时 VM 探索，因为行动开始前就被当前状态边界挡下了。下一步先查看自己的状态、spark、day/week 余量、疲劳和睡眠情况，再决定工作或休息。"
		case stoppedReason == "duration_window_too_short":
			return "本次运行在实时探索轮次开始前结束。时间窗口太短，不适合安全消耗一次真实模型调用，因此没有执行 VM 动作。"
		default:
			return "本次运行没有发生实时 VM 探索。resident 在停止前没有进入有效 action 轮次。"
		}
	}
	if strings.HasPrefix(stoppedReason, "upstream_request_failed:") {
		return fmt.Sprintf("本次运行在下一次模型调用被可重试 upstream 请求失败打断前，完成了 %d 个有效轮次。已记录的 round log 仍是 resident 在中断前观察和行动的有效证据。", len(rounds))
	}
	if strings.Contains(stoppedReason, "final_reflection_failed") || strings.Contains(stoppedReason, "acceptance_failed") {
		return fmt.Sprintf("本次运行完成了 %d 个有效轮次，但最终回顾调用失败。这个 partial report 以已记录 round log 为事实来源。", len(rounds))
	}
	return ""
}
