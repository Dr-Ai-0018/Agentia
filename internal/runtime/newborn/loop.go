package newborn

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/context"
	"ai-arena/internal/memory"
	"ai-arena/internal/openai"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

type Runner struct {
	client   *http.Client
	baseURL  string
	apiKey   string
	actions  ActionExecutor
	budget   *BudgetController
	reports  *ReportWriter
	world    *WorldBridge
	memories *memory.FileStore
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
	RunGroupID       string
	RecentActions    []RecentAction
}

func NewRunner(client *http.Client, baseURL, apiKey string) *Runner {
	return &Runner{
		client:   client,
		baseURL:  baseURL,
		apiKey:   apiKey,
		actions:  NewIncusActionExecutor(),
		budget:   NewBudgetController(broker.New(".agents")),
		reports:  NewReportWriter(),
		world:    NewWorldBridge(".agents"),
		memories: memory.NewFileStore(".agents/memory"),
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
				"Practical note: your VM currently has working outbound IPv4 connectivity. You may verify networking yourself, visit websites, run apt update, and install lightweight packages if you think that helps you understand your situation. " +
				"If you discover a need that clearly requires a host-side decision, such as more resources, a policy clarification, or an environment change, you may open a formal ticket instead of relying only on casual chat.",
		},
	}
	state := loopState{
		UsedActions: map[string]int{},
		NotePath:    "/root/arena-notes/boot-notes.md",
		RunGroupID:  fmt.Sprintf("newborn-%s-%s", profile.Name, started.Format("20060102T150405Z")),
	}
	roundLogs := []RoundLog{}
	stoppedReason := ""
	round := 0

	for {
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

		prepared, err := r.budget.Preflight(profile, state, roundNow)
		if err != nil {
			return FinalReport{}, fmt.Errorf("round %d preflight failed: %w", round, err)
		}
		if prepared != nil && prepared.Denied {
			stoppedReason = fmt.Sprintf("broker_preflight_denied: %s", strings.Join(prepared.DeniedReason, ","))
			break
		}

		packet := r.buildContextPacket(profile, remaining, state)
		input := append([]openai.Message(nil), history...)
		input = append(input, openai.Message{
			Role:    "user",
			Content: packet.FullInput(),
		})

		result, err := openai.PostStream(r.client, r.baseURL, r.apiKey, buildDecisionToolPayload(profile, input, packet.PromptCacheKey(profile.Name)), verbose)
		if err != nil {
			return FinalReport{}, fmt.Errorf("round %d request failed: %w", round, err)
		}

		brokerLog, err := r.budget.Settle(profile, result, roundNow, runtimeguard.CallKindWork, tokenledger.ActivityNormalWork)
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

func (r *Runner) buildContextPacket(profile ResidentProfile, remaining int, state loopState) context.Packet {
	worldView := r.world.BuildResidentWorldView(profile, 6)
	memoryDigest := r.buildResidentMemoryDigest(profile)
	working := context.WorkingContext{
		RemainingSeconds: remaining,
		UsedActions:      state.UsedActions,
		NoopStreak:       state.NoopStreak,
		NotePath:         state.NotePath,
		LastObservation:  state.LastObservation,
		RecentActions:    renderRecentActions(state.RecentActions),
		FrontierStatus:   renderExplorationFrontier(state),
		BudgetFacts:      renderBudgetFacts(state),
		MemoryReview:     r.renderMemoryReviewQueue(profile),
		FreshWorldUpdates: worldView.FreshDeliveredItems,
	}
	if state.LastDecision != nil {
		working.LastSituation = state.LastDecision.Situation
		working.LastReason = state.LastDecision.Reason
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
		return context.MemoryDigest{
			Identity:     fmt.Sprintf("%s is newly awakened and is still building a first-person understanding of self through observation, choice, and consequences.", profile.Name),
			Resource:     "Known starting home envelope: 1 vCPU, 2 GiB RAM, 12 GiB disk, with actual details to be verified from inside the VM.",
			Relationship: "Chenglin is a separate human in the same world; the relationship is still mostly unformed.",
			Lessons:      "Earliest rule: observe first, then speak with evidence.",
			Strategy:     "Use first-hand inspection to build options; escalate outward only when the need is real and specific.",
			Governance:   []string{"memory_review_queue: none visible yet"},
		}
	}

	byDomain := map[memory.Domain][]string{}
	governance := []string{}
	for _, record := range records {
		if record.Status == memory.StatusDeleted {
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
		Identity:     joinDigestLines(byDomain[memory.DomainIdentity], fmt.Sprintf("%s is still shaping identity through direct action.", profile.Name)),
		Resource:     joinDigestLines(byDomain[memory.DomainResources], "Known starting home envelope: 1 vCPU, 2 GiB RAM, 12 GiB disk."),
		Relationship: joinDigestLines(byDomain[memory.DomainRelationships], "Chenglin is a separate human in the same world; the relationship is still forming through contact and consequences."),
		Lessons:      joinDigestLines(byDomain[memory.DomainLessons], "No durable lesson has outranked direct observation yet."),
		Strategy:     joinDigestLines(append([]string{}, byDomain[memory.DomainRules]...), "Use first-hand inspection to build options; escalate outward only when the need is real and specific."),
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
		return []string{"memory_review_queue: none visible right now"}
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
		reason = "This looks too close to a raw log excerpt."
	}
	if reason == "" && memoryLooksLikeLegacyDirective(record.Summary, record.ResidentText) {
		reason = "This memory still reads like an old system-written directive rather than a resident-owned note."
	}
	if reason == "" {
		reason = "Resident review is pending."
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

func (r *Runner) renderMemoryReviewQueue(profile ResidentProfile) []string {
	records, err := r.memories.ListAbstractMemories(profile.Name)
	if err != nil {
		return nil
	}
	lines := []string{}
	for _, record := range records {
		if !needsGovernanceFlag(record) {
			continue
		}
		lines = append(lines, renderGovernanceLine(record))
		if len(lines) >= 3 {
			break
		}
	}
	return lines
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
	out := make([]string, 0, len(order)+1)
	for _, surface := range order {
		status := "unseen"
		if surfaces[surface] {
			status = "seen"
		}
		out = append(out, fmt.Sprintf("%s=%s cost=%s", surface, status, surfaceCost(surface)))
	}
	if next, ok := nextUnexploredSurface(surfaces, budgetTier(state)); ok {
		out = append(out, fmt.Sprintf("next_preferred_surface=%s", next))
	}
	if baselineCaptureComplete(surfaces) {
		out = append(out, "baseline_capture_complete=true")
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

func baselineCaptureComplete(surfaces map[ExplorationSurface]bool) bool {
	return surfaces[SurfaceIdentity] && surfaces[SurfaceFilesystem] && surfaces[SurfaceResources] && surfaces[SurfaceNetwork]
}

func renderBudgetFacts(state loopState) []string {
	tier := budgetTier(state)
	out := []string{"budget_tier=" + tier}
	if state.LastBrokerUsage == nil {
		out = append(out,
			"budget_status=not_observed_yet",
			"next_call_cost_estimate=bootstrap_range",
		)
		return out
	}
	out = append(out,
		fmt.Sprintf("spark_balance_before=%.4f", state.LastBrokerUsage.BeforeSpark),
		fmt.Sprintf("spark_balance_after=%.4f", state.LastBrokerUsage.AfterSpark),
		fmt.Sprintf("last_call_spark_delta=%.4f", state.LastBrokerUsage.SparkDelta),
		fmt.Sprintf("last_call_strain_cost=%d", state.LastBrokerUsage.PreparedStrainCost),
		fmt.Sprintf("window_6h_usage=%d", state.LastBrokerUsage.Window6HUsed),
		fmt.Sprintf("day_usage=%d", state.LastBrokerUsage.DayUsed),
		fmt.Sprintf("week_usage=%d", state.LastBrokerUsage.WeekUsed),
		fmt.Sprintf("debt_active_before=%t", state.LastBrokerUsage.BeforeDebtActive),
		fmt.Sprintf("debt_active_after=%t", state.LastBrokerUsage.AfterDebtActive),
	)
	if state.LastBrokerUsage.AfterStatus != nil {
		status := state.LastBrokerUsage.AfterStatus
		effectiveWindow := status.Physiology.EffectiveWindow6HCap
		if effectiveWindow <= 0 {
			effectiveWindow = status.Window6HCap
		}
		effectiveDay := status.Physiology.EffectiveDayCap
		if effectiveDay <= 0 {
			effectiveDay = status.DayCap
		}
		effectiveWeek := status.Physiology.EffectiveWeekCap
		if effectiveWeek <= 0 {
			effectiveWeek = status.WeekCap
		}
		windowRemaining := status.Physiology.Window6HRemaining
		if windowRemaining <= 0 && effectiveWindow >= status.Window6HUsed {
			windowRemaining = effectiveWindow - status.Window6HUsed
		}
		dayRemaining := status.Physiology.DayRemaining
		if dayRemaining <= 0 && effectiveDay >= status.DayUsed {
			dayRemaining = effectiveDay - status.DayUsed
		}
		weekRemaining := status.Physiology.WeekRemaining
		if weekRemaining <= 0 && effectiveWeek >= status.WeekUsed {
			weekRemaining = effectiveWeek - status.WeekUsed
		}
		out = append(out,
			fmt.Sprintf("window_6h_remaining=%d", windowRemaining),
			fmt.Sprintf("day_remaining=%d", dayRemaining),
			fmt.Sprintf("week_remaining=%d", weekRemaining),
			fmt.Sprintf("window_6h_cap=%d", status.Window6HCap),
			fmt.Sprintf("day_cap=%d", status.DayCap),
			fmt.Sprintf("week_cap=%d", status.WeekCap),
			fmt.Sprintf("effective_window_6h_cap=%d", effectiveWindow),
			fmt.Sprintf("effective_day_cap=%d", effectiveDay),
			fmt.Sprintf("effective_week_cap=%d", effectiveWeek),
			fmt.Sprintf("next_recovery_at=%s", status.NextRecoveryAt),
			fmt.Sprintf("recovery_tick_minutes=%d", status.RecoveryTickMinutes),
			fmt.Sprintf("debt_amount=%.4f", status.DebtAmount),
			fmt.Sprintf("resident_mode=%s", status.Physiology.Mode),
			fmt.Sprintf("resident_pressure=%s", status.Physiology.Pressure),
			fmt.Sprintf("quota_tightest_layer=%s", status.Physiology.QuotaTightestLayer),
			fmt.Sprintf("quota_tightest_ratio=%.4f", status.Physiology.QuotaTightestRatio),
			fmt.Sprintf("recovery_suggested=%t", status.Physiology.RecoverySuggested),
			fmt.Sprintf("recovery_urgency=%s", status.Physiology.RecoveryUrgency),
		)
		for _, line := range status.Physiology.SummaryLines {
			out = append(out, "physiology_summary="+line)
		}
	}
	if next := projectedNextCallFacts(state); len(next) > 0 {
		out = append(out, next...)
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
		fmt.Sprintf("next_call_estimated_input_tokens=%d", estimatedInput),
		fmt.Sprintf("next_call_estimated_cached_tokens=%d", estimatedCached),
		fmt.Sprintf("next_call_estimated_output_tokens=%d", estimatedOutput),
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
			SurfaceResources,
			SurfaceNetwork,
			SurfaceWorld,
			SurfaceServices,
			SurfacePackages,
		}
	case "balanced":
		return []ExplorationSurface{
			SurfaceIdentity,
			SurfaceFilesystem,
			SurfaceResources,
			SurfaceNetwork,
			SurfaceServices,
			SurfaceWorld,
			SurfacePackages,
		}
	default:
		return []ExplorationSurface{
			SurfaceIdentity,
			SurfaceFilesystem,
			SurfaceResources,
			SurfaceNetwork,
			SurfaceServices,
			SurfacePackages,
			SurfaceWorld,
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
		switch {
		case strings.HasPrefix(stoppedReason, "broker_preflight_denied:"):
			return "No live VM exploration occurred in this run because the resident was blocked by broker preflight before any model call was made. The next move is to inspect the resident runtime state, funding, reserve policy, and 6h quota budget before retrying."
		case stoppedReason == "duration_window_too_short":
			return "This run ended before a live exploration round could begin. The time window was too short to spend a real model call safely, so no VM action was taken."
		default:
			return "No live VM exploration occurred in this run. The resident did not reach a valid action round before the run stopped."
		}
	}
	return ""
}
