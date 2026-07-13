package newborn

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/brokerstate"
	"ai-arena/internal/context"
	"ai-arena/internal/memory"
	"ai-arena/internal/openai"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
	"ai-arena/internal/worldstate"
)

type fakeActionExecutor struct {
	result    ActionResult
	calls     *int
	decisions *[]AgentDecision
}

func (f fakeActionExecutor) Execute(_ ResidentProfile, decision AgentDecision) ActionResult {
	if f.calls != nil {
		*f.calls++
	}
	if f.decisions != nil {
		*f.decisions = append(*f.decisions, decision)
	}
	return f.result
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRenderQuotaObservationIsCompact(t *testing.T) {
	out := broker.QuotaOutput{
		Status: brokerstate.ResidentStatus{
			ResidentID:     "jade",
			SparkBalance:   2.625,
			DebtActive:     false,
			DebtAmount:     0,
			RecoveryMode:   "idle",
			LastRecoveryAt: time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC),
		},
		Quota: brokerstate.QuotaSnapshot{
			Window6HRemaining:          530,
			EffectiveWindow6HRemaining: 497,
			DayRemaining:               2020,
			EffectiveDayRemaining:      1880,
			WeekRemaining:              12340,
			EffectiveWeekRemaining:     11780,
			WorkAllowedNow:             true,
			RecoveryMode:               "idle",
			NextRecoveryAt:             "2026-06-07T09:15:00Z",
		},
	}

	got := renderQuotaObservation(out)
	if strings.Contains(got, "{") || strings.Contains(got, "quota\":") {
		t.Fatalf("expected compact text observation, got %q", got)
	}
	for _, want := range []string{
		"self quota 快照:",
		"quota_model=rolling_day_week_with_recent_6h_pace",
		"resident_id=jade",
		"spark_balance=2.6250",
		"recent_6h_remaining_reference=",
		"rolling_day_remaining=",
		"can_work_now=true",
		"next_natural_recovery_at=2026-06-07T09:15:00Z",
		"sleep_hint=如果最近节奏太快",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
	for _, banned := range []string{"effective_window_6h", "window_6h=", "6h 额度紧张"} {
		if strings.Contains(got, banned) {
			t.Fatalf("self quota observation should not contain legacy wording %q in %q", banned, got)
		}
	}
}

func TestRenderResidentStatusObservationIsCompact(t *testing.T) {
	status := brokerstate.ResidentStatus{
		ResidentID:           "amber",
		SparkBalance:         4.125,
		Fatigue:              180,
		SleepDebt:            1,
		DebtActive:           false,
		DebtAmount:           0,
		RecoveryMode:         "rest",
		Window6HCap:          720,
		Window6HUsed:         190,
		EffectiveWindow6HCap: 690,
		DayCap:               2400,
		DayUsed:              410,
		EffectiveDayCap:      2280,
		WeekCap:              16800,
		WeekUsed:             1100,
		EffectiveWeekCap:     16000,
		NextRecoveryAt:       "2026-06-07T10:00:00Z",
		LastRecoveryAt:       time.Date(2026, 6, 7, 9, 45, 0, 0, time.UTC),
	}
	got := renderResidentStatusObservation(status)
	if strings.Contains(got, "{") || strings.Contains(got, "\"resident_id\"") {
		t.Fatalf("expected compact text observation, got %q", got)
	}
	for _, want := range []string{
		"self status 快照:",
		"resident_id=amber",
		"spark_balance=4.1250",
		"recent_6h_used=190",
		"rolling_day_remaining=2400",
		"next_natural_recovery_at=2026-06-07T10:00:00Z",
		"last_recovery_at=2026-06-07T09:45:00Z",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
	for _, banned := range []string{"effective_window_6h", "window_6h=", "effective_day_cap", "quota_tightest"} {
		if strings.Contains(got, banned) {
			t.Fatalf("self status observation should not contain legacy wording %q in %q", banned, got)
		}
	}
}

func TestBuildProfile(t *testing.T) {
	profile, err := BuildProfile("amber")
	if err != nil {
		t.Fatalf("build profile: %v", err)
	}
	if profile.Model != "gpt-5.4" {
		t.Fatalf("unexpected model: %s", profile.Model)
	}
}

func TestPromptAndWorldContextTreatChatAsAsyncPeerRelationship(t *testing.T) {
	instructions := makeInstructions()
	for _, want := range []string{
		"不是 owner/assistant、主人/副手或雇佣关系",
		"pending 只表示程林尚未回复",
		"没有新的程林回复或没有新的用户请求，不是选择 noop 的充分理由",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("expected instruction %q in %q", want, instructions)
		}
	}

	world := NewWorldBridge(t.TempDir())
	view := world.BuildResidentWorldView(ResidentProfile{Name: "jade"}, 10)
	for _, want := range []string{
		"chat_mode: 自由、异步",
		"pending 只是异步消息状态，不是暂停、等待命令或停止探索的理由",
		"你和程林不是主人/副手或雇佣关系",
	} {
		if !strings.Contains(view.RenderedChat, want) {
			t.Fatalf("expected world context %q in %q", want, view.RenderedChat)
		}
	}
}

func TestPreflightSpecBootstrap(t *testing.T) {
	spec := preflightSpec(ResidentProfile{Name: "amber", Model: "gpt-5.5"}, loopState{}, time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC), 0)
	if spec.Usage.InputTokens != 1100 {
		t.Fatalf("unexpected bootstrap input: %d", spec.Usage.InputTokens)
	}
	if spec.Usage.OutputTokens != 260 {
		t.Fatalf("unexpected bootstrap output: %d", spec.Usage.OutputTokens)
	}
}

func TestPreflightSpecFromLastUsage(t *testing.T) {
	spec := preflightSpec(ResidentProfile{Name: "jade", Model: "gpt-5.4"}, loopState{
		LastRealUsage: &openai.StreamResult{
			InputTokens:  1000,
			CachedTokens: 200,
			OutputTokens: 300,
		},
	}, time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC), 0)
	if spec.Usage.InputTokens != 1050 {
		t.Fatalf("unexpected estimated input tokens: %d", spec.Usage.InputTokens)
	}
	if spec.Usage.CachedTokens != 204 {
		t.Fatalf("unexpected cached tokens: %d", spec.Usage.CachedTokens)
	}
	if spec.Usage.OutputTokens != 315 {
		t.Fatalf("unexpected estimated output tokens: %d", spec.Usage.OutputTokens)
	}
}

func TestPreflightSpecUsesMeasuredPromptWhenLarger(t *testing.T) {
	spec := preflightSpec(ResidentProfile{Name: "jade", Model: "gpt-5.4"}, loopState{
		LastRealUsage: &openai.StreamResult{
			InputTokens:  1000,
			CachedTokens: 200,
			OutputTokens: 300,
		},
	}, time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC), 4800)
	if spec.Usage.InputTokens != 4800 {
		t.Fatalf("expected measured prompt tokens to drive preflight, got %d", spec.Usage.InputTokens)
	}
	if spec.Usage.CachedTokens != 204 {
		t.Fatalf("expected cached estimate to still use prior cache observation, got %d", spec.Usage.CachedTokens)
	}
	if spec.Usage.ResponseID != "preflight_estimate_measured_prompt" {
		t.Fatalf("expected measured response marker, got %q", spec.Usage.ResponseID)
	}
}

func TestPreflightSpecUsesLastActionActivityShape(t *testing.T) {
	spec := preflightSpec(ResidentProfile{Name: "jade", Model: "gpt-5.4"}, loopState{
		LastDecision: &AgentDecision{
			NextAction: "guest_exec",
			Command:    "whoami",
		},
		LastRealUsage: &openai.StreamResult{
			InputTokens:  1000,
			CachedTokens: 200,
			OutputTokens: 100,
		},
	}, time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC), 0)
	if spec.Activity != tokenledger.ActivityLightWork {
		t.Fatalf("expected light work preflight for narrow probe, got %s", spec.Activity)
	}
}

func TestRecoveryModeForPreflightUsesRestAfterSleep(t *testing.T) {
	if got := recoveryModeForPreflight(loopState{}); got != "idle" {
		t.Fatalf("expected idle without previous sleep, got %q", got)
	}
	if got := recoveryModeForPreflight(loopState{LastDecision: &AgentDecision{NextAction: "self_quota"}}); got != "idle" {
		t.Fatalf("expected idle after non-sleep action, got %q", got)
	}
	if got := recoveryModeForPreflight(loopState{LastDecision: &AgentDecision{NextAction: "sleep", SleepMinutes: 12}}); got != "rest" {
		t.Fatalf("expected rest after sleep, got %q", got)
	}
}

func TestClassifyGuestExecActivityNarrowProbe(t *testing.T) {
	if got := classifyGuestExecActivity("uname -a"); got != tokenledger.ActivityLightWork {
		t.Fatalf("expected light work for narrow probe, got %s", got)
	}
	if got := classifyGuestExecActivity("hostnamectl; uname -a"); got != tokenledger.ActivityNormalWork {
		t.Fatalf("expected normal work for bundled probe, got %s", got)
	}
}

func TestExecuteWriteNoteRejectsCommandOnlyDecisionWithRawOutput(t *testing.T) {
	executor := &IncusActionExecutor{}
	result := executor.Execute(ResidentProfile{Name: "onyx", Instance: "onyx"}, AgentDecision{
		NextAction: "write_note",
		Command:    "cat >> /root/arena-notes/boot-notes.md <<'EOF'\nunsafe\nEOF",
		Reason:     "old malformed note style",
	})
	if !result.Error {
		t.Fatalf("expected action error")
	}
	if result.ErrorKind != "validation_error" {
		t.Fatalf("unexpected error kind: %s", result.ErrorKind)
	}
	if !strings.Contains(result.RawOutput, "cat >> /root/arena-notes/boot-notes.md") {
		t.Fatalf("expected raw malformed command in raw output, got %q", result.RawOutput)
	}
}

func TestValidateGuestExecRejectsRiskyContinuityHeredoc(t *testing.T) {
	command := "cat >> /root/arena-notes/boot-notes.md <<'EOF'\nunsafe\nEOF"
	result, denied := validateGuestExecCommand(command)
	if !denied {
		t.Fatalf("expected risky continuity heredoc to be denied")
	}
	if !result.Error || result.ErrorKind != "continuity_surface_requires_note_tool" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !strings.Contains(result.Observation, "note_replace_with_backup") {
		t.Fatalf("expected note tool guidance, got %q", result.Observation)
	}
	if !strings.Contains(result.Observation, "语义上的工具选择错误") {
		t.Fatalf("expected semantic error guidance, got %q", result.Observation)
	}
	if !strings.Contains(result.RawOutput, "cat >> /root/arena-notes/boot-notes.md") {
		t.Fatalf("expected raw command for debugging, got %q", result.RawOutput)
	}
}

func TestValidateGuestExecRejectsContinuityRead(t *testing.T) {
	command := "cat /root/arena-notes/boot-notes.md"
	result, denied := validateGuestExecCommand(command)
	if !denied {
		t.Fatalf("expected continuity read to require note_read")
	}
	if result.ErrorKind != "continuity_surface_requires_note_tool" {
		t.Fatalf("unexpected error kind: %s", result.ErrorKind)
	}
	if !strings.Contains(result.Observation, "note_read") {
		t.Fatalf("expected note_read guidance, got %q", result.Observation)
	}
}

func TestValidateGuestExecRejectsSedContinuityEdit(t *testing.T) {
	command := "sed -i 's/old/new/' /root/arena-notes/boot-notes.md"
	result, denied := validateGuestExecCommand(command)
	if !denied {
		t.Fatalf("expected sed continuity edit to be denied")
	}
	if result.ErrorKind != "continuity_surface_requires_note_tool" {
		t.Fatalf("unexpected error kind: %s", result.ErrorKind)
	}
}

func TestLimitRawOutputAddsTruncationMarker(t *testing.T) {
	raw := strings.Repeat("x", actionRawOutputMax+10)
	got := limitRawOutput(raw)
	if !strings.HasPrefix(got, strings.Repeat("x", actionRawOutputMax)) {
		t.Fatalf("expected raw output prefix to be preserved")
	}
	if !strings.Contains(got, "raw_output_truncated bytes_omitted=10") {
		t.Fatalf("expected truncation marker, got suffix %q", got[len(got)-80:])
	}
}

func TestLimitActionObservationCapsSuccessfulOutput(t *testing.T) {
	raw := "\n" + strings.Repeat("x", actionRawOutputMax+10) + "\n"
	got := limitActionObservation(raw)
	if !strings.HasPrefix(got, strings.Repeat("x", actionRawOutputMax)) {
		t.Fatalf("expected observation prefix to be preserved")
	}
	if !strings.Contains(got, "raw_output_truncated bytes_omitted=10") {
		t.Fatalf("expected truncation marker, got suffix %q", got[len(got)-80:])
	}
	if strings.HasPrefix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Fatalf("expected outer whitespace to be trimmed")
	}
}

func TestFallbackAcceptance(t *testing.T) {
	got := fallbackAcceptance(nil, "broker_preflight_denied")
	if got == "" {
		t.Fatalf("expected fallback acceptance text")
	}
	partial := fallbackAcceptance([]RoundLog{{Round: 1}}, "upstream_request_failed: round_2")
	if !strings.Contains(partial, "1 个有效轮次") || !strings.Contains(partial, "upstream 请求失败") {
		t.Fatalf("expected partial upstream fallback, got %q", partial)
	}
}

func TestRenderAcceptanceRoundRecap(t *testing.T) {
	rounds := []RoundLog{
		{
			Round: 1,
			Decision: AgentDecision{
				NextAction: "guest_exec",
				Command:    "hostname",
			},
			Observation: "jade\n",
		},
		{
			Round: 2,
			Decision: AgentDecision{
				NextAction: "talk_to_chenglin",
				Message:    "I found the machine quiet but healthy.",
			},
			Observation: "message delivered to Chenglin",
		},
	}

	got := renderAcceptanceRoundRecap(rounds)
	if !strings.Contains(got, "round=1 action=guest_exec") {
		t.Fatalf("expected guest_exec recap, got %q", got)
	}
	if !strings.Contains(got, "command=hostname") {
		t.Fatalf("expected command recap, got %q", got)
	}
	if !strings.Contains(got, "round=2 action=talk_to_chenglin") {
		t.Fatalf("expected chat recap, got %q", got)
	}
	if !strings.Contains(got, "message=I found the machine quiet but healthy.") {
		t.Fatalf("expected message recap, got %q", got)
	}
}

func TestParseDecisionResultFromFunctionCall(t *testing.T) {
	result := openai.StreamResult{
		FunctionCalls: []openai.ResponseItem{
			{
				Type:      "function_call",
				Name:      "decide_next_action",
				Arguments: `{"situation":"fresh boot","next_action":"guest_exec","reason":"inspect first","command":"whoami","message":""}`,
			},
		},
	}

	decision, err := parseDecisionResult(result)
	if err != nil {
		t.Fatalf("parse decision result: %v", err)
	}
	if decision.NextAction != "guest_exec" {
		t.Fatalf("unexpected next action: %s", decision.NextAction)
	}
	if decision.Command != "whoami" {
		t.Fatalf("unexpected command: %s", decision.Command)
	}
}

func TestParseDecisionResultFromSplitToolCallName(t *testing.T) {
	result := openai.StreamResult{
		FunctionCalls: []openai.ResponseItem{
			{
				Type:      "function_call",
				CallName:  "guest_exec",
				Arguments: `{"situation":"Need an identity probe.","reason":"Observe before assuming.","command":"whoami"}`,
			},
		},
	}

	decision, err := parseDecisionResult(result)
	if err != nil {
		t.Fatalf("parse decision result: %v", err)
	}
	if decision.NextAction != "guest_exec" || decision.Command != "whoami" {
		t.Fatalf("unexpected split-tool decision: %#v", decision)
	}
}

func TestParseDecisionResultSupportsSelfQuota(t *testing.T) {
	result := openai.StreamResult{
		FunctionCalls: []openai.ResponseItem{
			{
				Type:      "function_call",
				Name:      "decide_next_action",
				Arguments: `{"situation":"I want exact quota facts before deciding whether to work or rest.","next_action":"self_quota","reason":"broker state is more accurate than guessing from shell output","command":"","message":"","ticket_title":"","ticket_body":"","ticket_priority":"","memory_id":"","memory_action":"","memory_summary":"","memory_text":"","memory_layer":"","memory_reason":""}`,
			},
		},
	}

	decision, err := parseDecisionResult(result)
	if err != nil {
		t.Fatalf("parse decision result: %v", err)
	}
	if decision.NextAction != "self_quota" {
		t.Fatalf("unexpected next action: %s", decision.NextAction)
	}
	if decision.Command != "" {
		t.Fatalf("expected self_quota command to be cleared, got %q", decision.Command)
	}
}

func TestParseDecisionResultSupportsSleep(t *testing.T) {
	result := openai.StreamResult{
		FunctionCalls: []openai.ResponseItem{
			{
				Type:      "function_call",
				CallName:  "sleep",
				Arguments: `{"situation":"My recent pace is high and a background command can continue without another model call.","reason":"Rest for a short interval instead of spending another active turn.","sleep_minutes":12,"command":"echo no"}`,
			},
		},
	}

	decision, err := parseDecisionResult(result)
	if err != nil {
		t.Fatalf("parse sleep decision result: %v", err)
	}
	if decision.NextAction != "sleep" {
		t.Fatalf("unexpected next action: %s", decision.NextAction)
	}
	if decision.SleepMinutes != 12 {
		t.Fatalf("expected sleep minutes to survive normalization, got %d", decision.SleepMinutes)
	}
	if decision.Command != "" {
		t.Fatalf("expected sleep command to be cleared, got %q", decision.Command)
	}
}

func TestParseDecisionResultMapsLegacyWriteNoteToNoteAppend(t *testing.T) {
	result := openai.StreamResult{
		FunctionCalls: []openai.ResponseItem{
			{
				Type:      "function_call",
				Name:      "decide_next_action",
				Arguments: `{"situation":"I want to preserve a local continuity fact.","next_action":"write_note","reason":"Plain note text is enough.","command":"cat >> /root/arena-notes/boot-notes.md <<'EOF'\nunsafe\nEOF","message":"","ticket_title":"","ticket_body":"","ticket_priority":"","memory_id":"","memory_action":"","memory_summary":"","memory_text":"I confirmed the note surface exists.","memory_layer":"","memory_reason":""}`,
			},
		},
	}

	decision, err := parseDecisionResult(result)
	if err != nil {
		t.Fatalf("parse decision result: %v", err)
	}
	if decision.NextAction != "note_append" {
		t.Fatalf("unexpected next action: %s", decision.NextAction)
	}
	if decision.Command != "" {
		t.Fatalf("expected legacy write_note command to be cleared, got %q", decision.Command)
	}
	if decision.NoteText != "I confirmed the note surface exists." {
		t.Fatalf("expected memory_text to move into note_text, got %q", decision.NoteText)
	}
	if decision.MemoryText != "" {
		t.Fatalf("expected memory_text to be cleared after normalization, got %q", decision.MemoryText)
	}
}

func TestParseDecisionResultRejectsWriteNoteWithoutMemoryTextAndRecordsRawArguments(t *testing.T) {
	result := openai.StreamResult{
		FunctionCalls: []openai.ResponseItem{
			{
				Type:      "function_call",
				Name:      "decide_next_action",
				Arguments: `{"situation":"I want to write a note.","next_action":"write_note","reason":"Malformed old-style note command.","command":"cat >> /root/arena-notes/boot-notes.md <<'EOF'\nthis should not be accepted\nEOF","message":"","ticket_title":"","ticket_body":"","ticket_priority":"","memory_id":"","memory_action":"","memory_summary":"","memory_text":"","memory_layer":"","memory_reason":""}`,
			},
		},
	}

	_, err := parseDecisionResult(result)
	if err == nil {
		t.Fatalf("expected write_note without memory_text to fail validation")
	}
	if !strings.Contains(err.Error(), "raw_arguments=") {
		t.Fatalf("expected raw arguments in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "cat >> /root/arena-notes/boot-notes.md") {
		t.Fatalf("expected original malformed command in error, got %v", err)
	}
}

func TestParseDecisionResultClearsCommandForNoop(t *testing.T) {
	result := openai.StreamResult{
		FunctionCalls: []openai.ResponseItem{
			{
				Type:      "function_call",
				Name:      "decide_next_action",
				Arguments: `{"situation":"Nothing urgent is pending.","next_action":"noop","reason":"Wait for new information before acting.","command":"echo should-not-run"}`,
			},
		},
	}

	decision, err := parseDecisionResult(result)
	if err != nil {
		t.Fatalf("parse decision result: %v", err)
	}
	if decision.NextAction != "noop" {
		t.Fatalf("unexpected next action: %s", decision.NextAction)
	}
	if decision.Command != "" {
		t.Fatalf("expected noop command to be cleared, got %q", decision.Command)
	}
}

func TestParseDecisionResultRejectsGuestExecWithoutCommand(t *testing.T) {
	result := openai.StreamResult{
		FunctionCalls: []openai.ResponseItem{
			{
				Type:      "function_call",
				Name:      "decide_next_action",
				Arguments: `{"situation":"Need to inspect the VM.","next_action":"guest_exec","reason":"Run one narrow probe.","command":""}`,
			},
		},
	}

	if _, err := parseDecisionResult(result); err == nil {
		t.Fatalf("expected guest_exec without command to fail validation")
	}
}

func TestParseDecisionResultSupportsSplitNoteAppendTool(t *testing.T) {
	result := openai.StreamResult{
		FunctionCalls: []openai.ResponseItem{
			{
				Type:      "function_call",
				Name:      "note_append",
				Arguments: `{"situation":"Need continuity.","reason":"Record concise local fact.","note_file":"","note_text":"I confirmed the safe note API is the continuity path."}`,
			},
		},
	}

	decision, err := parseDecisionResult(result)
	if err != nil {
		t.Fatalf("parse decision result: %v", err)
	}
	if decision.NextAction != "note_append" {
		t.Fatalf("unexpected next action: %s", decision.NextAction)
	}
	if decision.Command != "" {
		t.Fatalf("expected split note tool not to carry command, got %q", decision.Command)
	}
	if decision.NoteText != "I confirmed the safe note API is the continuity path." {
		t.Fatalf("unexpected note text: %q", decision.NoteText)
	}
}

func TestParseDecisionResultSupportsNoteCompactTool(t *testing.T) {
	result := openai.StreamResult{
		FunctionCalls: []openai.ResponseItem{
			{
				Type:      "function_call",
				Name:      "note_summarize_or_compact",
				Arguments: `{"situation":"Boot notes are too long.","reason":"Compact after reading them.","note_file":"boot-notes.md","note_text":"Compact continuity summary."}`,
			},
		},
	}

	decision, err := parseDecisionResult(result)
	if err != nil {
		t.Fatalf("parse decision result: %v", err)
	}
	if decision.NextAction != "note_summarize_or_compact" {
		t.Fatalf("unexpected next action: %s", decision.NextAction)
	}
	if decision.Command != "" {
		t.Fatalf("expected compact note tool not to carry command, got %q", decision.Command)
	}
	if decision.NoteText != "Compact continuity summary." {
		t.Fatalf("unexpected note text: %q", decision.NoteText)
	}
}

func TestCompactObservationForHistory(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 120; i++ {
		b.WriteString("line\n")
	}

	compacted := compactObservationForHistory(b.String())
	if !strings.Contains(compacted, "[observation truncated for context reuse:") {
		t.Fatalf("expected truncation marker, got %q", compacted)
	}
	if len(compacted) > maxObservationHistoryChars+200 {
		t.Fatalf("compacted observation too large: %d", len(compacted))
	}
}

func TestBuildContextPacketStableCacheKeyAcrossWorkingShift(t *testing.T) {
	runner := NewRunner(nil, "", "")
	profile := ResidentProfile{
		Name:     "amber",
		Model:    "gpt-5.5",
		Persona:  "coordinator",
		Style:    "clear",
		CoreBias: "reduce confusion",
	}
	packetA := runner.buildContextPacket(profile, 300, loopState{
		UsedActions: map[string]int{"guest_exec": 1},
		NotePath:    "/root/arena-notes/boot-notes.md",
	})
	packetB := runner.buildContextPacket(profile, 120, loopState{
		UsedActions:     map[string]int{"guest_exec": 2, "talk_to_chenglin": 1},
		NotePath:        "/root/arena-notes/boot-notes.md",
		LastObservation: "machine state changed",
	})
	if packetA.PromptCacheKey(profile.Name) != packetB.PromptCacheKey(profile.Name) {
		t.Fatalf("working changes should not change prompt cache key")
	}
	if packetA.FullInput() == packetB.FullInput() {
		t.Fatalf("working changes should change full packet")
	}
}

func TestDecisionPayloadPlacesStableContextAsFirstInputMessage(t *testing.T) {
	runner := NewRunner(nil, "", "")
	profile := ResidentProfile{
		Name:     "amber",
		Model:    "gpt-5.5",
		Persona:  "coordinator",
		Style:    "clear",
		CoreBias: "reduce confusion",
	}
	packet := runner.buildContextPacket(profile, 240, loopState{
		UsedActions:     map[string]int{"guest_exec": 1},
		NotePath:        "/root/arena-notes/boot-notes.md",
		LastObservation: "dynamic observation",
	})
	history := []openai.Message{
		{Role: "user", Content: "initial"},
		{Role: "assistant", Content: "previous decision"},
	}
	history = appendWorkingContext(history, packet)
	input := buildDecisionInput(packet.StablePrefix(), history)
	payload := buildDecisionToolPayload(profile, input, packet.PromptCacheKey(profile.Name))

	if payload.Instructions != makeInstructions() {
		t.Fatalf("instructions must stay fixed tool rules only")
	}
	if strings.Contains(payload.Instructions, "[system_const]") ||
		strings.Contains(payload.Instructions, "[world_state]") ||
		strings.Contains(payload.Instructions, "[memory_digest]") {
		t.Fatalf("stable resident context must not be promoted into instructions: %q", payload.Instructions)
	}
	if len(payload.Input) != 4 {
		t.Fatalf("expected stable prefix plus append-only history, got %#v", payload.Input)
	}
	if !strings.Contains(payload.Input[0].Content, "[system_const]") ||
		!strings.Contains(payload.Input[0].Content, "[world_state]") ||
		!strings.Contains(payload.Input[0].Content, "[memory_digest]") {
		t.Fatalf("stable context must be first input message: %q", payload.Input[0].Content)
	}
	if strings.Contains(payload.Input[0].Content, "[recent_working_context]") {
		t.Fatalf("dynamic working context must not enter stable prefix")
	}
	if payload.Input[1].Content != "initial" || payload.Input[2].Content != "previous decision" {
		t.Fatalf("history order was changed: %#v", payload.Input)
	}
	if !strings.Contains(payload.Input[len(payload.Input)-1].Content, "[recent_working_context]") {
		t.Fatalf("dynamic working context must be the final input message: %#v", payload.Input)
	}
}

func TestRunHistoryBuildsThreeSegmentInputWithSummaryPane(t *testing.T) {
	history := newRunHistoryForPurpose("", 60)
	history.summaryPane = &SummaryPane{
		Text:           "我前面确认了机器状态，还留了一个待继续的小问题。",
		UpdatedAt:      "2026-07-13T12:00:00Z",
		RoundsAbsorbed: 4,
		ApproxTokens:   32,
		EvidenceRefs: []SummaryPaneEvidenceRef{{
			Kind:   "note",
			Ref:    "boot-notes.md",
			Rounds: []int{1, 2},
		}},
	}
	history.recent = append(history.recent, openai.Message{Role: "user", Content: "[recent_working_context]\nremaining_seconds=120"})

	input := history.input("[stable prefix]")
	if len(input) != 4 {
		t.Fatalf("expected stable prefix, summary pane, preamble, recent context; got %#v", input)
	}
	if input[0].Content != "[stable prefix]" {
		t.Fatalf("stable prefix must stay first, got %#v", input)
	}
	if !strings.Contains(input[1].Content, "[older_round_summary_pane]") ||
		!strings.Contains(input[1].Content, "boot-notes.md") {
		t.Fatalf("expected summary pane with evidence refs in second segment, got %q", input[1].Content)
	}
	if !strings.Contains(input[3].Content, "[recent_working_context]") {
		t.Fatalf("expected recent verbatim window after summary/preamble, got %#v", input)
	}
}

func TestRunHistorySummaryPaneSnapshotIsDeepCopy(t *testing.T) {
	history := newRunHistoryForPurpose("", 0)
	history.summaryPane = &SummaryPane{
		Text: "原始摘要",
		EvidenceRefs: []SummaryPaneEvidenceRef{{
			Kind:   "round",
			Ref:    "round-1",
			Rounds: []int{1},
		}},
	}

	snapshot := history.summaryPaneSnapshot()
	snapshot.Text = "改过的摘要"
	snapshot.EvidenceRefs[0].Rounds[0] = 99

	if history.summaryPane.Text != "原始摘要" || history.summaryPane.EvidenceRefs[0].Rounds[0] != 1 {
		t.Fatalf("summary pane snapshot must be detached from runtime state: %#v", history.summaryPane)
	}
	if history.recentRoundWindowLimit() != defaultCompactionRecentRounds {
		t.Fatalf("expected default recent round limit")
	}
}

func TestRunnerDecisionRequestPlacesStablePrefixBeforeHistory(t *testing.T) {
	dir := t.TempDir()
	var decisionPayloads []openai.RequestPayload
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		var payload openai.RequestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		if strings.HasPrefix(payload.PromptCacheKey, "arena-ctx-") {
			decisionPayloads = append(decisionPayloads, payload)
		}
		header := http.Header{}
		header.Set("Content-Type", "text/event-stream")
		var body string
		switch requests {
		case 1:
			body = renderSSECompleted(t, map[string]any{
				"id": "resp-decision",
				"usage": map[string]any{
					"input_tokens":  100,
					"output_tokens": 20,
				},
				"output": []map[string]any{
					{
						"type":      "function_call",
						"name":      "guest_exec",
						"arguments": `{"situation":"Need one direct fact.","reason":"Check identity.","command":"whoami"}`,
					},
				},
			})
		case 2:
			body = renderSSECompleted(t, map[string]any{
				"id": "resp-decision-2",
				"usage": map[string]any{
					"input_tokens":  120,
					"output_tokens": 20,
				},
				"output": []map[string]any{
					{
						"type":      "function_call",
						"name":      "noop",
						"arguments": `{"situation":"Enough for now.","reason":"Stop."}`,
					},
				},
			})
		case 3:
			body = renderSSECompleted(t, map[string]any{
				"id":          "resp-acceptance",
				"output_text": "I stopped after one decision.",
				"usage": map[string]any{
					"input_tokens":  80,
					"output_tokens": 20,
				},
			})
		default:
			t.Fatalf("unexpected request %d", requests)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}
	runner := NewRunner(client, "http://example.test", "test-key")
	runner.actions = fakeActionExecutor{result: ActionResult{Observation: "guest observed full output\nline two", Activity: tokenledger.ActivityStatusCheck}}
	runner.budget = NewBudgetController(broker.New(filepath.Join(dir, "agents")))
	runner.world = NewWorldBridge(filepath.Join(dir, "agents"))
	runner.memories = memory.NewFileStore(filepath.Join(dir, "agents", "memory"))

	_, err := runner.Run(ResidentProfile{Name: "jade", Model: "gpt-5.4", Instance: "jade"}, 2*time.Minute, filepath.Join(dir, "runs"), false, true)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(decisionPayloads) != 2 {
		t.Fatalf("expected two decision payloads, got %d", len(decisionPayloads))
	}
	first := decisionPayloads[0]
	second := decisionPayloads[1]
	if first.Instructions != makeInstructions() || second.Instructions != makeInstructions() {
		t.Fatalf("decision instructions must stay fixed")
	}
	if strings.Contains(first.Instructions, "[system_const]") || strings.Contains(second.Instructions, "[memory_digest]") {
		t.Fatalf("resident context leaked into instructions")
	}
	if first.PromptCacheKey != second.PromptCacheKey {
		t.Fatalf("prompt cache key changed within run: %q vs %q", first.PromptCacheKey, second.PromptCacheKey)
	}
	if len(first.Input) < 3 || len(second.Input) <= len(first.Input) {
		t.Fatalf("expected second input to append to first input, first=%#v second=%#v", first.Input, second.Input)
	}
	if !strings.Contains(first.Input[0].Content, "[system_const]") ||
		!strings.Contains(first.Input[0].Content, "[world_state]") ||
		!strings.Contains(first.Input[0].Content, "[memory_digest]") {
		t.Fatalf("stable context must be first input message: %q", first.Input[0].Content)
	}
	if first.Input[0].Content != second.Input[0].Content {
		t.Fatalf("stable prefix changed across turns")
	}
	if !messagesEqual(first.Input, second.Input[:len(first.Input)]) {
		t.Fatalf("second request did not preserve first request as byte-stable prefix\nfirst=%#v\nsecond_prefix=%#v", first.Input, second.Input[:len(first.Input)])
	}
	last := second.Input[len(second.Input)-1].Content
	if !strings.Contains(last, "[recent_working_context]") {
		t.Fatalf("expected latest working context as final input, got %q", last)
	}
	if !strings.Contains(second.Input[len(first.Input)].Content, "Function call returned by model:") ||
		!strings.Contains(second.Input[len(first.Input)].Content, `"command":"whoami"`) {
		t.Fatalf("expected raw function call arguments appended to history, got %q", second.Input[len(first.Input)].Content)
	}
	if !strings.Contains(second.Input[len(first.Input)+1].Content, "guest observed full output") {
		t.Fatalf("expected full observation appended to history, got %q", second.Input[len(first.Input)+1].Content)
	}
}

func TestRunnerRetriesOnceAfterContextOverflowWithSilentTrim(t *testing.T) {
	dir := t.TempDir()
	requests := 0
	contextRetrySeen := false
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		var payload openai.RequestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		header := http.Header{}
		header.Set("Content-Type", "text/event-stream")
		switch requests {
		case 1, 2:
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body: io.NopCloser(strings.NewReader(renderSSECompleted(t, map[string]any{
					"id": fmt.Sprintf("resp-decision-%d", requests),
					"usage": map[string]any{
						"input_tokens":  100 + requests,
						"output_tokens": 20,
					},
					"output": []map[string]any{{
						"type":      "function_call",
						"name":      "guest_exec",
						"arguments": `{"situation":"Need another observed fact.","reason":"Continue.","command":"whoami"}`,
					}},
				}))),
				Request: r,
			}, nil
		case 3:
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"context_length_exceeded","message":"maximum context length exceeded"}}`)),
				Request:    r,
			}, nil
		case 4:
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body: io.NopCloser(strings.NewReader(renderSSECompleted(t, map[string]any{
					"id":          "resp-compaction",
					"output_text": "我确认过两个事实，现在可以收住这轮。",
					"usage": map[string]any{
						"input_tokens":  60,
						"cached_tokens": 40,
						"output_tokens": 12,
					},
				}))),
				Request: r,
			}, nil
		case 5:
			contextRetrySeen = true
			if len(payload.Input) >= 8 {
				t.Fatalf("expected retry input to be trimmed, got %d messages", len(payload.Input))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body: io.NopCloser(strings.NewReader(renderSSECompleted(t, map[string]any{
					"id": "resp-decision-retry",
					"usage": map[string]any{
						"input_tokens":  80,
						"output_tokens": 10,
					},
					"output": []map[string]any{{
						"type":      "function_call",
						"name":      "noop",
						"arguments": `{"situation":"Enough for this run.","reason":"Stop after retry."}`,
					}},
				}))),
				Request: r,
			}, nil
		case 6:
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body: io.NopCloser(strings.NewReader(renderSSECompleted(t, map[string]any{
					"id":          "resp-acceptance",
					"output_text": "Recovered from a context overflow retry and stopped cleanly.",
					"usage": map[string]any{
						"input_tokens":  70,
						"output_tokens": 12,
					},
				}))),
				Request: r,
			}, nil
		default:
			t.Fatalf("unexpected request %d", requests)
		}
		return nil, nil
	})}
	runner := NewRunner(client, "http://example.test", "test-key")
	runner.actions = fakeActionExecutor{result: ActionResult{Observation: "observed", Activity: tokenledger.ActivityStatusCheck}}
	runner.budget = NewBudgetController(broker.New(filepath.Join(dir, "agents")))
	runner.world = NewWorldBridge(filepath.Join(dir, "agents"))
	runner.memories = memory.NewFileStore(filepath.Join(dir, "agents", "memory"))
	runner.SetRunOptions(RunOptions{CompactionRecentRounds: 2})

	report, err := runner.Run(ResidentProfile{Name: "jade", Model: "gpt-5.4", Instance: "jade"}, 3*time.Minute, filepath.Join(dir, "runs"), false, true)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !contextRetrySeen || requests != 6 {
		t.Fatalf("expected one context overflow retry and acceptance, requests=%d retry=%v", requests, contextRetrySeen)
	}
	if report.Rounds != 3 || report.StoppedReason != "resident_noop" {
		t.Fatalf("expected recovered run to stop by noop after three rounds, got %#v", report)
	}
}

func TestRunnerPreflightCompactionInstallsSummaryPane(t *testing.T) {
	dir := t.TempDir()
	requests := 0
	compactionRequestSeen := false
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		var payload openai.RequestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		header := http.Header{}
		header.Set("Content-Type", "text/event-stream")
		switch requests {
		case 1, 2:
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body: io.NopCloser(strings.NewReader(renderSSECompleted(t, map[string]any{
					"id": fmt.Sprintf("resp-decision-%d", requests),
					"usage": map[string]any{
						"input_tokens":  120,
						"output_tokens": 20,
					},
					"output": []map[string]any{{
						"type":      "function_call",
						"name":      "guest_exec",
						"arguments": `{"situation":"Need a little more local evidence.","reason":"Continue.","command":"whoami"}`,
					}},
				}))),
				Request: r,
			}, nil
		case 3:
			compactionRequestSeen = true
			last := payload.Input[len(payload.Input)-1].Content
			if !strings.Contains(last, "[inner_continuity_note]") {
				t.Fatalf("expected compaction natural directive, got %q", last)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body: io.NopCloser(strings.NewReader(renderSSECompleted(t, map[string]any{
					"id":          "resp-compact",
					"output_text": "我确认了本地身份线索，也看到连续两次检查都能正常返回。接下来可以收住，不必继续重复。",
					"usage": map[string]any{
						"input_tokens":  90,
						"output_tokens": 24,
					},
				}))),
				Request: r,
			}, nil
		case 4:
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body: io.NopCloser(strings.NewReader(renderSSECompleted(t, map[string]any{
					"id": "resp-decision-noop",
					"usage": map[string]any{
						"input_tokens":  100,
						"output_tokens": 14,
					},
					"output": []map[string]any{{
						"type":      "function_call",
						"name":      "noop",
						"arguments": `{"situation":"Enough for now.","reason":"Stop after preserving continuity."}`,
					}},
				}))),
				Request: r,
			}, nil
		case 5:
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body: io.NopCloser(strings.NewReader(renderSSECompleted(t, map[string]any{
					"id":          "resp-acceptance",
					"output_text": "The run compacted older rounds and stopped cleanly.",
					"usage": map[string]any{
						"input_tokens":  70,
						"output_tokens": 12,
					},
				}))),
				Request: r,
			}, nil
		default:
			t.Fatalf("unexpected request %d", requests)
		}
		return nil, nil
	})}
	runner := NewRunner(client, "http://example.test", "test-key")
	runner.actions = fakeActionExecutor{result: ActionResult{Observation: "observed", Activity: tokenledger.ActivityStatusCheck}}
	runner.budget = NewBudgetController(broker.New(filepath.Join(dir, "agents")))
	runner.world = NewWorldBridge(filepath.Join(dir, "agents"))
	runner.memories = memory.NewFileStore(filepath.Join(dir, "agents", "memory"))
	runner.SetRunOptions(RunOptions{CompactionRecentRounds: 1})

	report, err := runner.Run(ResidentProfile{Name: "jade", Model: "gpt-5.4", Instance: "jade"}, 3*time.Minute, filepath.Join(dir, "runs"), false, true)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !compactionRequestSeen || requests != 5 {
		t.Fatalf("expected preflight compaction request and acceptance, requests=%d seen=%v", requests, compactionRequestSeen)
	}
	if report.SummaryPane == nil || !strings.Contains(report.SummaryPane.Text, "本地身份线索") {
		t.Fatalf("expected summary pane in final report, got %#v", report.SummaryPane)
	}
	if len(report.CompactionEvents) != 1 || report.CompactionEvents[0].Outcome != CompactionOutcomeSummarized {
		t.Fatalf("expected summarized compaction event, got %#v", report.CompactionEvents)
	}
	if report.CompactionEvents[0].TriggerReason != CompactionTriggerPreflightMeasured || report.CompactionEvents[0].RoundsAbsorbed != 1 {
		t.Fatalf("unexpected compaction event: %#v", report.CompactionEvents[0])
	}
	if len(report.SummaryPane.EvidenceRefs) != 1 || report.SummaryPane.EvidenceRefs[0].Ref != "rounds_1_1" {
		t.Fatalf("expected structured round evidence ref, got %#v", report.SummaryPane.EvidenceRefs)
	}
}

func messagesEqual(a, b []openai.Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildDecisionToolPayloadUsesStableInstructions(t *testing.T) {
	payload := buildDecisionToolPayload(ResidentProfile{Name: "jade", Model: "gpt-5.4"}, []openai.Message{
		{Role: "user", Content: "hello"},
	}, "cache-key")
	if payload.Instructions != makeInstructions() {
		t.Fatalf("unexpected instructions payload")
	}
	if payload.PromptCacheKey != "cache-key" {
		t.Fatalf("unexpected prompt cache key")
	}
	if payload.ToolChoice != "required" {
		t.Fatalf("decision payload must require one tool call, got %#v", payload.ToolChoice)
	}
	if len(payload.Tools) < 10 {
		t.Fatalf("expected split action tools, got %#v", payload.Tools)
	}
	tools := map[string]openai.ResponseTool{}
	for _, item := range payload.Tools {
		tools[item.Name] = item
	}
	for _, name := range []string{"guest_exec", "note_list", "note_read", "note_append", "note_replace_with_backup", "note_restore_backup", "note_summarize_or_compact"} {
		if _, ok := tools[name]; !ok {
			t.Fatalf("missing split tool %q in %#v", name, tools)
		}
	}
	assertToolHasProperty(t, tools["guest_exec"], "command")
	assertToolMissingProperty(t, tools["guest_exec"], "note_text")
	for _, name := range []string{"note_list", "note_read", "note_append", "note_replace_with_backup", "note_restore_backup", "note_summarize_or_compact"} {
		assertToolMissingProperty(t, tools[name], "command")
	}
	assertToolHasProperty(t, tools["note_append"], "note_text")
	assertToolHasProperty(t, tools["note_replace_with_backup"], "note_text")
	assertToolHasProperty(t, tools["note_summarize_or_compact"], "note_text")
	assertToolHasProperty(t, tools["note_restore_backup"], "backup_file")
	assertToolHasProperty(t, tools["sleep"], "sleep_minutes")
}

func assertToolHasProperty(t *testing.T, tool openai.ResponseTool, name string) {
	t.Helper()
	properties, ok := tool.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool %s missing properties map: %#v", tool.Name, tool.Parameters)
	}
	if _, ok := properties[name]; !ok {
		t.Fatalf("tool %s missing property %s in %#v", tool.Name, name, properties)
	}
}

func assertToolMissingProperty(t *testing.T, tool openai.ResponseTool, name string) {
	t.Helper()
	properties, ok := tool.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool %s missing properties map: %#v", tool.Name, tool.Parameters)
	}
	if _, ok := properties[name]; ok {
		t.Fatalf("tool %s should not expose property %s in %#v", tool.Name, name, properties)
	}
}

func TestContextPackageDirectly(t *testing.T) {
	packet := context.Build(context.BuildSpec{
		Identity: context.ResidentIdentity{
			Name:     "jade",
			Model:    "gpt-5.4",
			Persona:  "steady engineer",
			Style:    "plain",
			CoreBias: "stability",
		},
		WorldState: "recent_chat: none recorded",
		MemoryDigest: context.MemoryDigest{
			Identity: "newborn",
		},
		Working: context.WorkingContext{
			RemainingSeconds: 200,
		},
	})
	if !strings.Contains(packet.FullInput(), "[system_const]") {
		t.Fatalf("missing system const section")
	}
	if !strings.Contains(packet.FullInput(), "[recent_working_context]") {
		t.Fatalf("missing working context section")
	}
}

func TestRecordRoundMemoryWritesHistoryGroupAndShortReflection(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(nil, "", "")
	runner.memories = memory.NewFileStore(filepath.Join(dir, "memory"))

	profile := ResidentProfile{
		Name:     "amber",
		Model:    "gpt-5.5",
		Persona:  "coordinator",
		Style:    "clear",
		CoreBias: "reduce confusion",
	}
	state := loopState{
		UsedActions: map[string]int{"guest_exec": 2},
		NotePath:    "/root/arena-notes/boot-notes.md",
		RunGroupID:  "newborn-amber-test-run",
	}
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	var err error
	state, err = runner.recordRoundMemory(profile, state, 2, AgentDecision{
		Situation:  "I should make contact while the machine picture is still fresh.",
		NextAction: "talk_to_chenglin",
		Reason:     "relationship matters early",
		Message:    "hello",
	}, "message delivered to Chenglin", now)
	if err != nil {
		t.Fatalf("record round memory: %v", err)
	}
	if state.LastReflectRound != 2 {
		t.Fatalf("expected last reflect round update, got %d", state.LastReflectRound)
	}

	groups, err := runner.memories.ListHistoryGroups(profile.Name)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 history group, got %d", len(groups))
	}
	if groups[0].EventCount != 1 {
		t.Fatalf("expected event count 1, got %d", groups[0].EventCount)
	}

	records, err := runner.memories.ListAbstractMemories(profile.Name)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 short reflection, got %d", len(records))
	}
	if records[0].Layer != memory.LayerShort {
		t.Fatalf("expected short layer, got %s", records[0].Layer)
	}
	if len(records[0].SourceGroupIDs) != 1 || records[0].SourceGroupIDs[0] != state.RunGroupID {
		t.Fatalf("expected source group id %q, got %#v", state.RunGroupID, records[0].SourceGroupIDs)
	}
}

func TestRunnerReportRecordsActionFailureRawOutput(t *testing.T) {
	dir := t.TempDir()
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		header := http.Header{}
		header.Set("Content-Type", "text/event-stream")
		header.Set("x-request-id", fmt.Sprintf("req-%d", requests))
		var body string
		switch requests {
		case 1:
			time.Sleep(2 * time.Second)
			body = renderSSECompleted(t, map[string]any{
				"id": "resp-decision",
				"usage": map[string]any{
					"input_tokens":  100,
					"output_tokens": 40,
				},
				"output": []map[string]any{
					{
						"type":      "function_call",
						"name":      "decide_next_action",
						"arguments": `{"situation":"Need a small probe.","next_action":"guest_exec","reason":"Check identity.","command":"whoami","message":"","ticket_title":"","ticket_body":"","ticket_priority":"","memory_id":"","memory_action":"","memory_summary":"","memory_text":"","memory_layer":"","memory_reason":""}`,
					},
				},
			})
		default:
			body = renderSSECompleted(t, map[string]any{
				"id":          "resp-acceptance",
				"output_text": "I attempted one probe and saw the failure clearly.",
				"usage": map[string]any{
					"input_tokens":  80,
					"output_tokens": 20,
				},
			})
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}

	runner := NewRunner(client, "http://example.test", "test-key")
	runner.actions = fakeActionExecutor{result: ActionResult{
		Observation: "guest command failed:\ncat: write error: No space left on device",
		Activity:    tokenledger.ActivityLightWork,
		Error:       true,
		ErrorKind:   "guest_command_failed",
		RawOutput:   "cat: write error: No space left on device",
	}}
	runner.budget = NewBudgetController(broker.New(filepath.Join(dir, "agents")))
	runner.world = NewWorldBridge(filepath.Join(dir, "agents"))
	runner.memories = memory.NewFileStore(filepath.Join(dir, "agents", "memory"))

	report, err := runner.Run(ResidentProfile{Name: "jade", Model: "gpt-5.4", Instance: "jade"}, 27*time.Second, filepath.Join(dir, "runs"), false, true)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(report.RoundLogs) != 1 {
		t.Fatalf("expected one round, got %d", len(report.RoundLogs))
	}
	round := report.RoundLogs[0]
	if !round.ActionError {
		t.Fatalf("expected action_error to be recorded")
	}
	if round.ErrorKind != "guest_command_failed" {
		t.Fatalf("unexpected error kind: %s", round.ErrorKind)
	}
	if !strings.Contains(round.RawOutput, "No space left on device") {
		t.Fatalf("expected raw output in report, got %q", round.RawOutput)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "runs", "jade-*", "report.json"))
	if err != nil {
		t.Fatalf("glob report: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one report file, got %#v", matches)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(raw), `"action_error": true`) || !strings.Contains(string(raw), `"raw_output": "cat: write error: No space left on device"`) {
		t.Fatalf("expected action error and raw output in report json: %s", raw)
	}
}

func TestRunnerStopsAfterResidentNoop(t *testing.T) {
	dir := t.TempDir()
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		header := http.Header{}
		header.Set("Content-Type", "text/event-stream")
		header.Set("x-request-id", fmt.Sprintf("req-%d", requests))
		var body string
		switch requests {
		case 1:
			body = renderSSECompleted(t, map[string]any{
				"id": "resp-decision",
				"usage": map[string]any{
					"input_tokens":  100,
					"output_tokens": 40,
				},
				"output": []map[string]any{
					{
						"type":      "function_call",
						"name":      "decide_next_action",
						"arguments": `{"situation":"Baseline is stable and waiting is better than probing.","next_action":"noop","reason":"Pause until there is new information.","command":"","message":"","ticket_title":"","ticket_body":"","ticket_priority":"","memory_id":"","memory_action":"","memory_summary":"","memory_text":"","memory_layer":"","memory_reason":""}`,
					},
				},
			})
		case 2:
			body = renderSSECompleted(t, map[string]any{
				"id":          "resp-acceptance",
				"output_text": "I chose to pause after confirming that no immediate action was useful.",
				"usage": map[string]any{
					"input_tokens":  80,
					"output_tokens": 20,
				},
			})
		default:
			t.Fatalf("unexpected request %d after resident noop", requests)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}

	runner := NewRunner(client, "http://example.test", "test-key")
	runner.actions = fakeActionExecutor{result: ActionResult{Observation: "no operation executed", Activity: tokenledger.ActivityStatusCheck}}
	runner.budget = NewBudgetController(broker.New(filepath.Join(dir, "agents")))
	runner.world = NewWorldBridge(filepath.Join(dir, "agents"))
	runner.memories = memory.NewFileStore(filepath.Join(dir, "agents", "memory"))

	report, err := runner.Run(ResidentProfile{Name: "jade", Model: "gpt-5.4", Instance: "jade"}, 2*time.Minute, filepath.Join(dir, "runs"), false, true)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.Rounds != 1 || len(report.RoundLogs) != 1 {
		t.Fatalf("expected one noop round, got %#v", report)
	}
	if report.StoppedReason != "resident_noop" {
		t.Fatalf("expected resident_noop stop, got %q", report.StoppedReason)
	}
	if requests != 2 {
		t.Fatalf("expected decision plus acceptance requests only, got %d", requests)
	}
	if report.AcceptanceBroker == nil || !report.AcceptanceBroker.Applied {
		t.Fatalf("expected applied acceptance broker settlement, got %#v", report.AcceptanceBroker)
	}
	if report.AcceptanceBroker.ApplyReason != "acceptance call via gpt-5.4" {
		t.Fatalf("expected acceptance charge reason, got %q", report.AcceptanceBroker.ApplyReason)
	}
	if report.AcceptanceBroker.AfterStatus == nil {
		t.Fatalf("expected acceptance after status")
	}
	if report.AcceptanceBroker.AfterStatus.FinalNoticeUsed {
		t.Fatalf("normal acceptance must not consume final notice state")
	}
}

func TestRunnerSleepDoesNotRequestModelDuringSleep(t *testing.T) {
	dir := t.TempDir()
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		header := http.Header{}
		header.Set("Content-Type", "text/event-stream")
		header.Set("x-request-id", fmt.Sprintf("req-%d", requests))
		var body string
		switch requests {
		case 1:
			body = renderSSECompleted(t, map[string]any{
				"id": "resp-decision",
				"usage": map[string]any{
					"input_tokens":  100,
					"output_tokens": 40,
				},
				"output": []map[string]any{
					{
						"type":      "function_call",
						"name":      "sleep",
						"arguments": `{"situation":"Quota is tight and sleeping is better than another probe.","reason":"Rest briefly without asking Chenglin or burning another model turn.","sleep_minutes":1}`,
					},
				},
			})
		case 2:
			body = renderSSECompleted(t, map[string]any{
				"id":          "resp-acceptance",
				"output_text": "I chose to sleep briefly to conserve budget.",
				"usage": map[string]any{
					"input_tokens":  80,
					"output_tokens": 20,
				},
			})
		default:
			t.Fatalf("unexpected model request during resident sleep: %d", requests)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}

	runner := NewRunner(client, "http://example.test", "test-key")
	runner.actions = fakeActionExecutor{result: ActionResult{Observation: "sleep scheduled", Activity: tokenledger.ActivityStatusCheck}}
	runner.budget = NewBudgetController(broker.New(filepath.Join(dir, "agents")))
	runner.world = NewWorldBridge(filepath.Join(dir, "agents"))
	runner.memories = memory.NewFileStore(filepath.Join(dir, "agents", "memory"))

	report, err := runner.Run(ResidentProfile{Name: "jade", Model: "gpt-5.4", Instance: "jade"}, 27*time.Second, filepath.Join(dir, "runs"), false, true)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.Rounds != 1 || len(report.RoundLogs) != 1 {
		t.Fatalf("expected one sleep round, got %#v", report)
	}
	if report.RoundLogs[0].Decision.NextAction != "sleep" {
		t.Fatalf("expected sleep action, got %#v", report.RoundLogs[0].Decision)
	}
	if report.RoundLogs[0].Decision.SleepMinutes != 1 {
		t.Fatalf("expected sleep_minutes to be preserved, got %#v", report.RoundLogs[0].Decision)
	}
	if !strings.Contains(report.RoundLogs[0].Observation, "sleep scheduled") {
		t.Fatalf("expected sleep observation in round log, got %q", report.RoundLogs[0].Observation)
	}
	if requests != 2 {
		t.Fatalf("expected decision plus acceptance requests only, got %d", requests)
	}
	store := brokerstate.New(filepath.Join(dir, "agents", "brokerstate"))
	sessions, _, err := store.LoadSleepSessions("jade")
	if err != nil {
		t.Fatalf("load sleep sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one recorded sleep session, got %d", len(sessions))
	}
	if sessions[0].ActualMinutes < 0 || sessions[0].Depth == "" {
		t.Fatalf("unexpected sleep session: %#v", sessions[0])
	}
}

func TestRunnerStopsWithoutGuestFallbackWhenDecisionToolCallMissing(t *testing.T) {
	dir := t.TempDir()
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		header := http.Header{}
		header.Set("Content-Type", "text/event-stream")
		header.Set("x-request-id", fmt.Sprintf("req-%d", requests))
		if requests != 1 {
			t.Fatalf("unexpected request after parse failure: %d", requests)
		}
		body := renderSSECompleted(t, map[string]any{
			"id":          "resp-empty-decision",
			"output_text": "",
			"usage": map[string]any{
				"input_tokens":  100,
				"output_tokens": 0,
			},
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}

	actionCalls := 0
	runner := NewRunner(client, "http://example.test", "test-key")
	runner.actions = fakeActionExecutor{
		result: ActionResult{Observation: "this must not execute", Activity: tokenledger.ActivityLightWork},
		calls:  &actionCalls,
	}
	runner.budget = NewBudgetController(broker.New(filepath.Join(dir, "agents")))
	runner.world = NewWorldBridge(filepath.Join(dir, "agents"))
	runner.memories = memory.NewFileStore(filepath.Join(dir, "agents", "memory"))

	report, err := runner.Run(ResidentProfile{Name: "jade", Model: "gpt-5.4", Instance: "jade"}, 2*time.Minute, filepath.Join(dir, "runs"), false, true)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if actionCalls != 0 {
		t.Fatalf("parse failure must not execute guest action, got calls=%d", actionCalls)
	}
	if report.StoppedReason != "structured_decision_parse_failed" {
		t.Fatalf("unexpected stopped reason: %q", report.StoppedReason)
	}
	if report.Rounds != 1 || len(report.RoundLogs) != 1 {
		t.Fatalf("expected one parse-failure round, got %#v", report)
	}
	round := report.RoundLogs[0]
	if !round.FallbackUsed || !round.ActionError || round.ErrorKind != "structured_decision_parse_failed" {
		t.Fatalf("expected structured parse failure round, got %#v", round)
	}
	if round.Decision.NextAction != "noop" || strings.TrimSpace(round.Decision.Command) != "" {
		t.Fatalf("parse failure decision must be non-executing noop, got %#v", round.Decision)
	}
	if !strings.Contains(round.Observation, "no supported action function call returned") {
		t.Fatalf("expected semantic parse error observation, got %q", round.Observation)
	}
	if report.AcceptanceBroker != nil {
		t.Fatalf("parse-failure run should not make acceptance call, got %#v", report.AcceptanceBroker)
	}
}

func renderSSECompleted(t *testing.T, response map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"type":     "response.completed",
		"response": response,
	})
	if err != nil {
		t.Fatalf("marshal sse: %v", err)
	}
	return fmt.Sprintf("data: %s\n\n", raw)
}

func TestBudgetControllerPreflightAutoRecoversToNow(t *testing.T) {
	dir := t.TempDir()
	app := broker.New(dir)
	controller := NewBudgetController(app)
	start := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)

	if err := controller.ResetResident("jade", start); err != nil {
		t.Fatalf("reset resident: %v", err)
	}
	if _, err := app.RunAdmitSpec("jade", broker.CallSpec{
		Kind:      runtimeguard.CallKindWork,
		Usage:     openaiUsage(300, 0, 120, "gpt-5.4", "preflight_recover_seed", start.Add(time.Minute)),
		Penalties: tokenledger.Penalties{},
		Activity:  tokenledger.ActivityNormalWork,
	}, true); err != nil {
		t.Fatalf("seed admit: %v", err)
	}

	prepared, err := controller.Preflight(ResidentProfile{Name: "jade", Model: "gpt-5.4"}, loopState{}, start.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if prepared.BeforeStatus.LastRecoveryAt.Before(start.Add(30 * time.Minute)) {
		t.Fatalf("expected last recovery at to advance before preflight")
	}
}

func TestBuildResidentMemoryDigestReadsStoredMemories(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(nil, "", "")
	runner.memories = memory.NewFileStore(filepath.Join(dir, "memory"))
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	err := runner.memories.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:        "jade-rule-1",
			Layer:     memory.LayerLong,
			Domain:    memory.DomainRules,
			Status:    memory.StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident:       "jade",
		Summary:        "Prefer narrow, reversible paths before escalating complexity.",
		DecisionAction: memory.ActionPromote,
	})
	if err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	digest := runner.buildResidentMemoryDigest(ResidentProfile{Name: "jade"})
	if !strings.Contains(digest.Strategy, "Prefer narrow, reversible paths") {
		t.Fatalf("expected stored strategy in digest, got %q", digest.Strategy)
	}
}

func TestBuildResidentMemoryDigestSkipsOperatorOnlyMemory(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(nil, "", "")
	runner.memories = memory.NewFileStore(filepath.Join(dir, "memory"))
	now := time.Date(2026, 6, 17, 7, 0, 0, 0, time.UTC)

	records := []memory.AbstractMemory{
		{
			Record: memory.Record{
				ID:        "jade-public-lesson",
				Layer:     memory.LayerLong,
				Domain:    memory.DomainLessons,
				Status:    memory.StatusActive,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Resident:   "jade",
			Summary:    "Visible lesson: check local evidence before asking outward.",
			Visibility: memory.VisibilityResidentPrivate,
		},
		{
			Record: memory.Record{
				ID:        "jade-operator-observation",
				Layer:     memory.LayerLong,
				Domain:    memory.DomainLessons,
				Status:    memory.StatusActive,
				CreatedAt: now.Add(time.Minute),
				UpdatedAt: now.Add(time.Minute),
			},
			Resident:   "jade",
			Summary:    "Hidden operator observation: host saw an audit-only detail.",
			Visibility: memory.VisibilityOperatorObservation,
			Governance: memory.GovernanceMeta{
				ReviewState:  "needs_resident_review",
				ReviewReason: "audit-only detail must not enter resident prompt",
			},
		},
		{
			Record: memory.Record{
				ID:        "jade-private-journal",
				Layer:     memory.LayerLong,
				Domain:    memory.DomainRelationships,
				Status:    memory.StatusActive,
				CreatedAt: now.Add(2 * time.Minute),
				UpdatedAt: now.Add(2 * time.Minute),
			},
			Resident:   "jade",
			Summary:    "Hidden private journal: unshared feeling.",
			Visibility: memory.VisibilityPrivateJournal,
		},
	}
	for _, record := range records {
		if err := runner.memories.UpsertAbstractMemory(record); err != nil {
			t.Fatalf("upsert memory: %v", err)
		}
	}

	digest := runner.buildResidentMemoryDigest(ResidentProfile{Name: "jade"})
	joined := strings.Join([]string{digest.Lessons, digest.Relationship, strings.Join(digest.Governance, "\n")}, "\n")
	if !strings.Contains(joined, "Visible lesson") {
		t.Fatalf("expected visible memory in digest, got %q", joined)
	}
	if strings.Contains(joined, "Hidden operator observation") || strings.Contains(joined, "Hidden private journal") || strings.Contains(joined, "audit-only detail") {
		t.Fatalf("expected hidden memory to stay out of resident digest, got %q", joined)
	}

	reviewState := loopState{
		RecentActions: []RecentAction{
			{Action: "guest_exec", Signature: "guest_exec: whoami hostname uname -a", Observation: "hostname kernel os-release"},
			{Action: "guest_exec", Signature: "guest_exec: ls -la / find /root", Observation: "arena-notes"},
			{Action: "guest_exec", Signature: "guest_exec: df -h free -h nproc", Observation: "memory disk cpu"},
			{Action: "guest_exec", Signature: "guest_exec: ip addr ip route resolv.conf curl", Observation: "network"},
		},
		UsedActions: map[string]int{"guest_exec": 4, "self_quota": 1},
	}
	queue := strings.Join(runner.renderMemoryReviewQueue(ResidentProfile{Name: "jade"}, reviewState), "\n")
	if strings.Contains(queue, "jade-operator-observation") || strings.Contains(queue, "audit-only detail") {
		t.Fatalf("expected hidden memory to stay out of resident review queue, got %q", queue)
	}
}

func TestRenderMemoryReviewQueueIncludesVisiblePendingSummary(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(nil, "", "")
	runner.memories = memory.NewFileStore(filepath.Join(dir, "memory"))
	now := time.Date(2026, 6, 20, 8, 50, 0, 0, time.UTC)
	for _, record := range []memory.AbstractMemory{
		{
			Record:   memory.Record{ID: "jade-visible-1", Layer: memory.LayerShort, Domain: memory.DomainLessons, Status: memory.StatusActive, CreatedAt: now, UpdatedAt: now},
			Resident: "jade",
			Summary:  "visible memory 1",
			Governance: memory.GovernanceMeta{
				ReviewState:  "needs_resident_review",
				ReviewReason: "resident should decide whether this still carries value",
			},
		},
		{
			Record:   memory.Record{ID: "jade-visible-2", Layer: memory.LayerShort, Domain: memory.DomainLessons, Status: memory.StatusActive, CreatedAt: now, UpdatedAt: now},
			Resident: "jade",
			Summary:  "visible memory 2",
			Governance: memory.GovernanceMeta{
				ReviewState:  "needs_resident_review",
				ReviewReason: "resident should decide whether this still carries value",
			},
		},
		{
			Record:     memory.Record{ID: "jade-hidden-operator", Layer: memory.LayerShort, Domain: memory.DomainLessons, Status: memory.StatusActive, CreatedAt: now, UpdatedAt: now},
			Resident:   "jade",
			Summary:    "hidden operator memory",
			Visibility: memory.VisibilityOperatorObservation,
			Governance: memory.GovernanceMeta{
				ReviewState:  "needs_resident_review",
				ReviewReason: "operator-only",
			},
		},
	} {
		if err := runner.memories.UpsertAbstractMemory(record); err != nil {
			t.Fatalf("upsert memory: %v", err)
		}
	}
	state := loopState{
		RecentActions: []RecentAction{
			{Action: "guest_exec", Signature: "guest_exec: whoami hostname uname -a", Observation: "hostname kernel os-release"},
			{Action: "guest_exec", Signature: "guest_exec: ls -la / find /root", Observation: "arena-notes"},
			{Action: "guest_exec", Signature: "guest_exec: df -h free -h nproc", Observation: "memory disk cpu"},
			{Action: "guest_exec", Signature: "guest_exec: ip addr ip route resolv.conf curl", Observation: "network"},
		},
		UsedActions: map[string]int{"guest_exec": 4, "self_status": 1},
	}

	queue := strings.Join(runner.renderMemoryReviewQueue(ResidentProfile{Name: "jade"}, state), "\n")
	if !strings.Contains(queue, "memory_review_queue_summary: visible_pending=2 showing=2") {
		t.Fatalf("expected visible pending summary excluding operator-only memory, got %q", queue)
	}
	if strings.Contains(queue, "jade-hidden-operator") || strings.Contains(queue, "operator-only") {
		t.Fatalf("expected operator-only memory to stay hidden, got %q", queue)
	}
}

func TestShouldDelayMemoryReviewDuringNewbornOrientation(t *testing.T) {
	if !shouldDelayMemoryReview(loopState{}) {
		t.Fatalf("expected delay with no recent actions")
	}

	state := loopState{
		RecentActions: []RecentAction{
			{Action: "guest_exec", Signature: "guest_exec: whoami hostname uname -a", Observation: "hostname kernel os-release"},
			{Action: "guest_exec", Signature: "guest_exec: ls -la / find /root", Observation: "arena-notes"},
			{Action: "guest_exec", Signature: "guest_exec: df -h free -h nproc", Observation: "memory disk cpu"},
			{Action: "guest_exec", Signature: "guest_exec: ip addr ip route resolv.conf curl", Observation: "network"},
		},
		UsedActions: map[string]int{"guest_exec": 4},
	}
	if !shouldDelayMemoryReview(state) {
		t.Fatalf("expected delay before self sensing has happened")
	}

	state.UsedActions["self_quota"] = 1
	if shouldDelayMemoryReview(state) {
		t.Fatalf("expected review queue to unlock after self sensing")
	}
}

func TestShortReflectionCooldownSkipsAdjacentRounds(t *testing.T) {
	runner := NewRunner(nil, "", "")
	state := loopState{
		LastReflectRound: 2,
		UsedActions:      map[string]int{"guest_exec": 2, "talk_to_chenglin": 1},
	}
	if runner.shouldCreateShortReflection(state, 4, AgentDecision{NextAction: "talk_to_chenglin"}, "message delivered") {
		t.Fatalf("expected cooldown to suppress reflection")
	}
	if !runner.shouldCreateShortReflection(state, 5, AgentDecision{NextAction: "talk_to_chenglin"}, "message delivered") {
		t.Fatalf("expected reflection after cooldown window")
	}
}

func TestCloseRunHistoryGroupClosesOpenGroup(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(nil, "", "")
	runner.memories = memory.NewFileStore(filepath.Join(dir, "memory"))
	profile := ResidentProfile{Name: "onyx"}
	state := loopState{RunGroupID: "newborn-onyx-20260606T120000Z"}
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	err := runner.memories.UpsertHistoryGroup(memory.HistoryGroup{
		GroupUUID:   "newborn-onyx-20260606T120000Z",
		Resident:    "onyx",
		CreatedAt:   now,
		LastEventAt: now,
		SourceKind:  "newborn_runtime_rounds",
		State:       memory.HistoryGroupOpen,
	})
	if err != nil {
		t.Fatalf("upsert group: %v", err)
	}
	if err := runner.closeRunHistoryGroup(profile, state, now.Add(5*time.Minute), "finished", 3); err != nil {
		t.Fatalf("close group: %v", err)
	}
	groups, err := runner.memories.ListHistoryGroups("onyx")
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 || groups[0].State != memory.HistoryGroupClosed {
		t.Fatalf("expected closed group, got %#v", groups)
	}
}

func TestTempDirSanity(t *testing.T) {
	dir := t.TempDir()
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("temp dir missing: %v", err)
	}
}

func TestSafeNoteFileRejectsPathsAndTraversal(t *testing.T) {
	if got, err := safeNoteFile(""); err != nil || got != "boot-notes.md" {
		t.Fatalf("expected empty note_file to default to boot-notes.md, got %q err=%v", got, err)
	}
	for _, value := range []string{"/root/arena-notes/boot-notes.md", "../boot-notes.md", "subdir/note.md", `subdir\note.md`, ".."} {
		if got, err := safeNoteFile(value); err == nil {
			t.Fatalf("expected %q to be rejected, got %q", value, got)
		}
	}
}

func TestRenderRecentActions(t *testing.T) {
	lines := renderRecentActions([]RecentAction{
		{
			Round:       2,
			Action:      "write_note",
			Intent:      "continuity_note_capture",
			Reason:      "preserve continuity",
			Observation: "duplicate action suppressed: similar note",
			Suppressed:  true,
		},
	})
	if len(lines) != 1 {
		t.Fatalf("unexpected rendered lines length: %d", len(lines))
	}
	if !strings.Contains(lines[0], "suppressed=true") {
		t.Fatalf("expected suppressed marker in %q", lines[0])
	}
	if !strings.Contains(lines[0], "intent=continuity_note_capture") {
		t.Fatalf("expected intent marker in %q", lines[0])
	}
}

func TestClassifyCommandIntentDetectsContinuityCaptureInsideGuestExec(t *testing.T) {
	intent := classifyCommandIntent(AgentDecision{
		NextAction: "guest_exec",
		Command:    "cat > /root/arena-notes/boot-notes.md <<'EOF'\n- Hostname: onyx\n- Kernel: Linux\n- Disk: 12G\n- Memory: 2G\n- Debian trixie\nEOF",
	})
	if intent != "continuity_note_capture" {
		t.Fatalf("expected continuity_note_capture, got %q", intent)
	}
}

func TestDetectExplorationSurfacesAndNextFrontier(t *testing.T) {
	actions := []RecentAction{
		{
			Action:      "guest_exec",
			Signature:   "guest_exec: uname -a && whoami && ls -la / && df -h && free -h",
			Intent:      "general_exec",
			Observation: "Linux host\nroot\nfilesystem\nmemory\ndisk",
		},
	}
	surfaces := detectExplorationSurfaces(actions)
	if !surfaces[SurfaceIdentity] || !surfaces[SurfaceFilesystem] || !surfaces[SurfaceResources] {
		t.Fatalf("expected identity/filesystem/resources to be seen: %#v", surfaces)
	}
	if surfaces[SurfaceNetwork] {
		t.Fatalf("expected network to remain unseen: %#v", surfaces)
	}
	next, ok := nextUnexploredSurface(surfaces, "balanced")
	if !ok || next != SurfaceWorld {
		t.Fatalf("expected next frontier world, got %q ok=%v", next, ok)
	}
	if preferredProbeShape(next) == "" {
		t.Fatalf("expected probe shape guidance for next frontier")
	}
}

func TestRenderExplorationFrontierReportsObservedContextWithoutSteering(t *testing.T) {
	state := loopState{
		RecentActions: []RecentAction{
			{Signature: "guest_exec: whoami hostname uname -a", Observation: "hostname kernel os-release"},
			{Signature: "guest_exec: ls -la / find /root", Observation: "arena-notes"},
			{Signature: "guest_exec: df -h free -h nproc", Observation: "memory disk cpu"},
			{Signature: "guest_exec: ip addr ip route resolv.conf curl", Observation: "network"},
		},
	}
	lines := renderExplorationFrontier(state)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "observed_local_context=") {
		t.Fatalf("expected observed context summary in %q", joined)
	}
	if strings.Contains(joined, "next_preferred_surface") || strings.Contains(joined, "next_probe_shape") || strings.Contains(joined, "baseline_capture_complete") {
		t.Fatalf("expected no resident-facing steering markers in %q", joined)
	}
}

func TestBudgetTierAffectsNextFrontier(t *testing.T) {
	surfaces := map[ExplorationSurface]bool{
		SurfaceIdentity:   true,
		SurfaceFilesystem: true,
		SurfaceResources:  true,
		SurfaceNetwork:    true,
	}
	nextTight, ok := nextUnexploredSurface(surfaces, "tight")
	if !ok || nextTight != SurfaceWorld {
		t.Fatalf("expected tight frontier to prefer world before heavier surfaces, got %q ok=%v", nextTight, ok)
	}
	nextComfortable, ok := nextUnexploredSurface(surfaces, "comfortable")
	if !ok || nextComfortable != SurfaceWorld {
		t.Fatalf("expected comfortable frontier to prefer world, got %q ok=%v", nextComfortable, ok)
	}
}

func TestBudgetTierClassification(t *testing.T) {
	state := loopState{
		LastBrokerUsage: &BrokerUsageLog{
			AfterStatus: &brokerstate.ResidentStatus{
				SparkBalance: 1.8,
				Window6HCap:  10000,
				Window6HUsed: 2000,
			},
		},
	}
	if got := budgetTier(state); got != "tight" {
		t.Fatalf("expected tight, got %q", got)
	}
}

func TestRenderBudgetFactsUsesFactsNotDirectives(t *testing.T) {
	state := loopState{
		LastRealUsage: &openai.StreamResult{
			InputTokens:  1000,
			CachedTokens: 300,
			OutputTokens: 200,
		},
		LastBrokerUsage: &BrokerUsageLog{
			BeforeSpark:        4.5,
			AfterSpark:         4.125,
			SparkDelta:         -0.375,
			PreparedSparkCost:  0.375,
			PreparedStrainCost: 920,
			Window6HUsed:       4800,
			DayUsed:            7200,
			WeekUsed:           11000,
			BeforeDebtActive:   false,
			AfterDebtActive:    false,
			Quota: &brokerstate.QuotaSnapshot{
				WorkAllowedNow:      true,
				NextRecoveryAt:      "2026-06-07T09:00:00Z",
				RecoveryTickMinutes: 15,
			},
			AfterStatus: &brokerstate.ResidentStatus{
				SparkBalance:        4.125,
				Fatigue:             800,
				SleepDebt:           3,
				RecoveryMode:        "idle",
				Window6HCap:         12000,
				Window6HUsed:        4800,
				DayCap:              60000,
				DayUsed:             7200,
				WeekCap:             150000,
				WeekUsed:            11000,
				NextRecoveryAt:      "2026-06-07T09:00:00Z",
				RecoveryTickMinutes: 15,
				DebtAmount:          0,
				Physiology: brokerstate.ResidentPhysiology{
					Mode:                 brokerstate.ModeFocused,
					Pressure:             "watchful",
					SparkBalance:         4.125,
					Fatigue:              800,
					SleepDebt:            3,
					EffectiveWindow6HCap: 12000,
					EffectiveDayCap:      60000,
					EffectiveWeekCap:     150000,
					Window6HRemaining:    7200,
					DayRemaining:         52800,
					WeekRemaining:        139000,
					QuotaTightestLayer:   "6h",
					QuotaTightestRatio:   0.6,
					RecoverySuggested:    false,
					RecoveryUrgency:      "none",
					SummaryLines: []string{
						"mode=focused",
						"pressure=watchful",
					},
				},
			},
		},
	}
	lines := renderBudgetFacts(state)
	joined := strings.Join(lines, "\n")
	for _, banned := range []string{"prefer ", "avoid ", "should ", "must "} {
		if strings.Contains(joined, banned) {
			t.Fatalf("budget facts should not contain directive %q in %q", banned, joined)
		}
	}
	for _, required := range []string{
		"spark_balance_after=4.1250",
		"recent_6h_cap_reference=12000",
		"recent_6h_remaining_reference=7200",
		"rolling_day_remaining=52800",
		"can_work_now=true",
		"recovery_mode=idle",
		"resident_mode=focused",
		"resident_pressure=watchful",
		"next_natural_recovery_at=2026-06-07T09:00:00Z",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("expected budget fact %q in %q", required, joined)
		}
	}
	for _, banned := range []string{"effective_window_6h", "effective_day_remaining", "work_allowed_now", "next_recovery_at"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("budget facts should not contain legacy wording %q in %q", banned, joined)
		}
	}
}

func TestSummarizeShortReflectionAvoidsDirectiveTone(t *testing.T) {
	for _, got := range []string{
		summarizeShortReflection(AgentDecision{NextAction: "talk_to_chenglin"}, ""),
		summarizeShortReflection(AgentDecision{NextAction: "submit_ticket"}, ""),
		summarizeShortReflection(AgentDecision{NextAction: "write_note"}, ""),
		summarizeShortReflection(AgentDecision{NextAction: "guest_exec"}, ""),
	} {
		lower := strings.ToLower(got)
		for _, banned := range []string{"should ", "must ", "need to "} {
			if strings.Contains(lower, banned) {
				t.Fatalf("reflection summary should avoid directive tone %q in %q", banned, got)
			}
		}
	}
}

func TestGuestExecFrontierExpansionCanTriggerShortReflection(t *testing.T) {
	runner := NewRunner(nil, "", "")
	state := loopState{
		RecentActions: []RecentAction{
			{
				Round:       1,
				Action:      "guest_exec",
				Signature:   "guest_exec: whoami && hostname",
				Intent:      "general_exec",
				Observation: "root amber",
			},
			{
				Round:       2,
				Action:      "guest_exec",
				Signature:   "guest_exec: ls -la / && df -h && free -h && ip addr",
				Intent:      "general_exec",
				Observation: "filesystem disk memory network",
			},
		},
		UsedActions: map[string]int{"guest_exec": 2},
	}
	if !runner.shouldCreateShortReflection(state, 2, AgentDecision{NextAction: "guest_exec"}, "filesystem disk memory network") {
		t.Fatalf("expected frontier expansion guest_exec to trigger short reflection")
	}
}

func TestCompressObservationFactsExtractsUsefulSignals(t *testing.T) {
	observation := `
uid=0(root) gid=0(root)
PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
hostname=amber
arena-notes
default via 10.244.206.1 dev enp5s0
2 packets transmitted, 2 received, 0% packet loss
HTTP/2 200
incus-agent.service loaded active running
`
	got := compressObservationFacts(observation)
	for _, want := range []string{
		"我确认了自己在 VM 内拥有 root 级控制。",
		"这台机器标识为 Debian 13 (trixie)。",
		"机器名解析为 amber。",
		"home 目录里已经存在本地笔记和连续性文件。",
		"VM 内能看到网络接口和默认路由。",
		"直接检查中 outbound IPv4、DNS 解析和 HTTPS 可达性都可用。",
		"核心 guest 服务仍在运行，包括 incus-agent 和 systemd networking。",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected compressed fact %q in %q", want, got)
		}
	}
}

func TestSummarizeReflectionFactsFallsBackToDecisionWhenObservationIsThin(t *testing.T) {
	decision := AgentDecision{
		NextAction: "guest_exec",
		Command:    "free -h && df -h && nproc && find /root/arena-notes -maxdepth 2 -type f",
		Situation:  "Resources are still unseen in this round and local notes need mapping.",
		Reason:     "Direct resource and notes inspection will sharpen the working picture.",
	}
	got := summarizeReflectionFacts(decision, "Appended resource/process snapshot to /root/arena-notes/boot-notes.md")
	for _, want := range []string{
		"我检查了内存、磁盘、CPU 或 uptime 等基础资源状态。",
		"我测绘了部分 home 目录和本地笔记表面。",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected fallback fact %q in %q", want, got)
		}
	}
}

func TestAssessMemoryGovernanceFlagsRawLogLikeMemory(t *testing.T) {
	meta := assessMemoryGovernance(
		"## Network snapshot - 2026-06-07T05:09:36Z ### IP addresses lo UNKNOWN 127.0.0.1/8 ::1/128 enp5s0 UP 10.244.206.102/24 /dev/sda2",
		"UTC 2026-06-07T05:09:46Z, round 3. ## Network snapshot - 2026-06-07T05:09:36Z ### IP addresses ... /dev/sda2 ...",
		time.Date(2026, 6, 7, 5, 20, 0, 0, time.UTC),
		false,
	)
	if meta.ReviewState != "needs_resident_review" {
		t.Fatalf("expected review state, got %#v", meta)
	}
	if meta.Quality != "low" {
		t.Fatalf("expected low quality, got %#v", meta)
	}
	for _, forbidden := range meta.HostMay {
		if forbidden == "delete" || forbidden == "rewrite" {
			t.Fatalf("host should not be allowed to %q directly", forbidden)
		}
	}
}

func TestBuildResidentMemoryDigestIncludesGovernanceQueue(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(nil, "", "")
	runner.memories = memory.NewFileStore(filepath.Join(dir, "memory"))
	now := time.Date(2026, 6, 7, 5, 20, 0, 0, time.UTC)

	err := runner.memories.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:        "amber-short-raw",
			Layer:     memory.LayerShort,
			Domain:    memory.DomainLessons,
			Status:    memory.StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident:     "amber",
		Summary:      "## Network snapshot - 2026-06-07T05:09:36Z ### IP addresses lo UNKNOWN 127.0.0.1/8 ::1/128 enp5s0 UP 10.244.206.102/24 /dev/sda2",
		ResidentText: "UTC 2026-06-07T05:09:46Z, round 3. ## Network snapshot - 2026-06-07T05:09:36Z ### IP addresses ... /dev/sda2 ...",
	})
	if err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	digest := runner.buildResidentMemoryDigest(ResidentProfile{Name: "amber"})
	if len(digest.Governance) == 0 {
		t.Fatalf("expected governance lines")
	}
	if !strings.Contains(strings.Join(digest.Governance, "\n"), "resident_options=keep|rewrite|compress|demote|delete") {
		t.Fatalf("expected resident governance options in %#v", digest.Governance)
	}
}

func openaiUsage(input, cached, output int, model, responseID string, startedAt time.Time) tokenledger.Usage {
	return tokenledger.Usage{
		InputTokens:  input,
		CachedTokens: cached,
		OutputTokens: output,
		TotalTokens:  input + output,
		Model:        model,
		ResponseID:   responseID,
		StartedAt:    startedAt,
		FinishedAt:   startedAt.Add(2 * time.Second),
	}
}

func TestLegacyDirectiveMemoryAlsoEntersGovernanceQueue(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(nil, "", "")
	runner.memories = memory.NewFileStore(filepath.Join(dir, "memory"))
	now := time.Date(2026, 6, 7, 6, 50, 0, 0, time.UTC)

	err := runner.memories.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:        "jade-short-legacy",
			Layer:     memory.LayerShort,
			Domain:    memory.DomainLessons,
			Status:    memory.StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident: "jade",
		Summary:  "A local note was updated; use it as the continuity anchor instead of re-deriving the same state from scratch.",
	})
	if err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	digest := runner.buildResidentMemoryDigest(ResidentProfile{Name: "jade"})
	joined := strings.Join(digest.Governance, "\n")
	if !strings.Contains(joined, "旧的系统写入指令") {
		t.Fatalf("expected legacy directive governance reason in %q", joined)
	}
}

func TestMemoryReviewRequestMapping(t *testing.T) {
	decision := AgentDecision{
		MemoryAction:  "rewrite",
		MemorySummary: "new summary",
		MemoryText:    "new text",
		MemoryLayer:   "short",
		MemoryReason:  "old version was too raw",
		Reason:        "I want a cleaner carry-forward note.",
	}
	req := decision.MemoryReviewRequest()
	if req.Action != memory.ActionUpdate {
		t.Fatalf("expected rewrite to map to update, got %s", req.Action)
	}
	decision.MemoryAction = "compress"
	if got := decision.MemoryReviewRequest().Action; got != memory.ActionSummarize {
		t.Fatalf("expected compress to map to summarize, got %s", got)
	}
	decision.MemoryAction = "demote"
	if got := decision.MemoryReviewRequest().Action; got != memory.ActionDecay {
		t.Fatalf("expected demote to map to decay, got %s", got)
	}
}

func TestNormalizeDecisionForActionClearsIrrelevantFields(t *testing.T) {
	decision := compactDecision(AgentDecision{
		NextAction:     "guest_exec",
		Command:        "whoami",
		Message:        "hello",
		TicketTitle:    "need thing",
		TicketBody:     "body",
		TicketPriority: "high",
		MemoryID:       "m1",
		MemoryAction:   "keep",
		MemorySummary:  "sum",
		MemoryText:     "text",
		MemoryLayer:    "short",
		MemoryReason:   "why",
	})
	if decision.Command == "" {
		t.Fatalf("expected command to remain for guest_exec")
	}
	if decision.Message != "" || decision.TicketTitle != "" || decision.MemoryID != "" {
		t.Fatalf("expected irrelevant fields cleared, got %#v", decision)
	}
}

func TestIncusActionExecutorMemoryReview(t *testing.T) {
	dir := t.TempDir()
	exec := &IncusActionExecutor{
		world:    NewWorldBridge(".agents"),
		memories: memory.NewFileStore(filepath.Join(dir, "memory")),
	}
	now := time.Date(2026, 6, 7, 6, 30, 0, 0, time.UTC)
	err := exec.memories.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:        "amber-short-legacy",
			Layer:     memory.LayerShort,
			Domain:    memory.DomainLessons,
			Status:    memory.StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident: "amber",
		Summary:  "raw legacy text",
	})
	if err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	result := exec.Execute(ResidentProfile{Name: "amber"}, AgentDecision{
		NextAction:    "memory_review",
		MemoryID:      "amber-short-legacy",
		MemoryAction:  "rewrite",
		MemorySummary: "I rewrote this into a cleaner carry-forward note.",
		MemoryReason:  "The previous version was too raw.",
		Reason:        "I want a useful short memory.",
	})
	if !strings.Contains(result.Observation, "memory review 已应用:") {
		t.Fatalf("expected memory review observation, got %q", result.Observation)
	}
	updated, ok, err := exec.memories.GetAbstractMemory("amber", "amber-short-legacy")
	if err != nil || !ok {
		t.Fatalf("get updated memory: ok=%v err=%v", ok, err)
	}
	if updated.Summary != "I rewrote this into a cleaner carry-forward note." {
		t.Fatalf("unexpected rewritten summary: %q", updated.Summary)
	}
}

func TestIncusActionExecutorSelfQuota(t *testing.T) {
	dir := t.TempDir()
	app := broker.New(filepath.Join(dir, ".agents"))
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)
	if _, err := app.RunReset("jade", now); err != nil {
		t.Fatalf("reset jade: %v", err)
	}

	exec := &IncusActionExecutor{
		world:    NewWorldBridge(filepath.Join(dir, ".agents")),
		memories: memory.NewFileStore(filepath.Join(dir, ".agents", "memory")),
		broker:   app,
	}
	result := exec.Execute(ResidentProfile{Name: "jade"}, AgentDecision{
		NextAction: "self_quota",
		Reason:     "exact broker quota facts are more reliable than shell inference",
	})
	if !strings.Contains(result.Observation, "self quota 快照:") {
		t.Fatalf("expected self quota snapshot, got %q", result.Observation)
	}
	if !strings.Contains(result.Observation, "resident_id=jade") {
		t.Fatalf("expected jade resident id in observation, got %q", result.Observation)
	}
	if !strings.Contains(result.Observation, "recovery_mode=") {
		t.Fatalf("expected recovery mode in observation, got %q", result.Observation)
	}
}

func TestReconcileReviewedMemoryArtifacts(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(nil, "", "")
	runner.memories = memory.NewFileStore(filepath.Join(dir, "memory"))
	now := time.Date(2026, 6, 7, 6, 40, 0, 0, time.UTC)
	err := runner.memories.UpsertAbstractMemory(memory.AbstractMemory{
		Record: memory.Record{
			ID:        "amber-short-resolved",
			Layer:     memory.LayerShort,
			Domain:    memory.DomainLessons,
			Status:    memory.StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Resident:     "amber",
		Summary:      "Clean resident-approved summary.",
		ResidentText: "## raw log tail /dev/sda2 uid=0(root) filesystem ...",
		Governance: memory.GovernanceMeta{
			ReviewState: "resolved",
		},
	})
	if err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	if err := runner.reconcileReviewedMemoryArtifacts(ResidentProfile{Name: "amber"}); err != nil {
		t.Fatalf("reconcile reviewed memories: %v", err)
	}
	updated, ok, err := runner.memories.GetAbstractMemory("amber", "amber-short-resolved")
	if err != nil || !ok {
		t.Fatalf("get memory: ok=%v err=%v", ok, err)
	}
	if updated.ResidentText != "Clean resident-approved summary." {
		t.Fatalf("expected resident text reconciliation, got %q", updated.ResidentText)
	}
}

func TestBuildResidentWorldContextConsumesRepliesWithoutReadMarkerExposure(t *testing.T) {
	dir := t.TempDir()
	world := NewWorldBridge(dir)
	profile := ResidentProfile{Name: "amber"}
	now := time.Date(2026, 6, 7, 7, 0, 0, 0, time.UTC)

	msg, err := world.store.AppendResidentToChenglin("amber", "hello", now)
	if err != nil {
		t.Fatalf("append resident message: %v", err)
	}
	reply, err := world.store.ReplyToResidentMessage(msg.ID, "reply", now.Add(time.Second))
	if err != nil {
		t.Fatalf("reply resident message: %v", err)
	}
	rendered := world.BuildResidentWorldContext(profile, 10)
	if strings.Contains(rendered, "status=read") || strings.Contains(rendered, "status=unread") {
		t.Fatalf("chat thread must not expose read markers, got %q", rendered)
	}
	if !strings.Contains(rendered, "status=delivered") {
		t.Fatalf("expected rendered thread to show delivered status, got %q", rendered)
	}
	thread, err := world.store.ReadThreadForResident("amber")
	if err != nil {
		t.Fatalf("read thread: %v", err)
	}
	found := false
	for _, item := range thread {
		if item.ID == reply.ID {
			found = true
			if item.Status != worldstate.StatusDelivered {
				t.Fatalf("expected persisted delivered status, got %s", item.Status)
			}
			if strings.TrimSpace(item.ReadAt) == "" {
				t.Fatalf("expected internal read_at marker to be set")
			}
		}
	}
	if !found {
		t.Fatalf("expected reply message in thread")
	}
}

func TestBuildResidentWorldViewReturnsFreshDeliveredItemsOnce(t *testing.T) {
	dir := t.TempDir()
	world := NewWorldBridge(dir)
	profile := ResidentProfile{Name: "amber"}
	now := time.Date(2026, 6, 7, 7, 0, 0, 0, time.UTC)

	msg, err := world.store.AppendResidentToChenglin("amber", "hello", now)
	if err != nil {
		t.Fatalf("append resident message: %v", err)
	}
	if _, err := world.store.ReplyToResidentMessage(msg.ID, "reply", now.Add(time.Second)); err != nil {
		t.Fatalf("reply resident message: %v", err)
	}

	first := world.BuildResidentWorldView(profile, 10)
	if len(first.FreshDeliveredItems) != 1 {
		t.Fatalf("expected 1 fresh delivered item, got %d", len(first.FreshDeliveredItems))
	}
	if !strings.Contains(first.FreshDeliveredItems[0], "reply") {
		t.Fatalf("expected fresh delivered item to contain reply, got %q", first.FreshDeliveredItems[0])
	}

	second := world.BuildResidentWorldView(profile, 10)
	if len(second.FreshDeliveredItems) != 0 {
		t.Fatalf("expected fresh delivered items to be consumed once, got %#v", second.FreshDeliveredItems)
	}
}

func TestBuildResidentWorldViewReturnsFreshTicketUpdatesOnce(t *testing.T) {
	dir := t.TempDir()
	world := NewWorldBridge(dir)
	profile := ResidentProfile{Name: "amber"}
	now := time.Date(2026, 6, 7, 7, 0, 0, 0, time.UTC)

	ticket, err := world.store.CreateResidentTicket("amber", "Need guidance", "Should I treat prior notes as canonical?", worldstate.TicketPriorityMedium, now)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := world.store.ReplyTicket(ticket.ID, "Yes, treat them as continuity unless contradicted.", false, now.Add(time.Second)); err != nil {
		t.Fatalf("reply ticket: %v", err)
	}

	first := world.BuildResidentWorldView(profile, 10)
	if !strings.Contains(first.RenderedChat, "recent_tickets:") {
		t.Fatalf("expected ticket block in rendered chat")
	}
	if len(first.FreshDeliveredItems) != 1 {
		t.Fatalf("expected 1 fresh ticket update, got %d", len(first.FreshDeliveredItems))
	}
	if !strings.Contains(first.FreshDeliveredItems[0], "ticket_update") {
		t.Fatalf("expected ticket_update marker, got %q", first.FreshDeliveredItems[0])
	}

	second := world.BuildResidentWorldView(profile, 10)
	if len(second.FreshDeliveredItems) != 0 {
		t.Fatalf("expected ticket fresh update to be consumed once, got %#v", second.FreshDeliveredItems)
	}
}

func TestBuildResidentWorldViewReturnsFreshHostInterventionsOnce(t *testing.T) {
	dir := t.TempDir()
	world := NewWorldBridge(dir)
	profile := ResidentProfile{Name: "amber"}
	now := time.Date(2026, 6, 7, 7, 0, 0, 0, time.UTC)

	if _, err := world.store.CreateHostIntervention("amber", "maintenance", "Planned maintenance", "A short maintenance window is scheduled.", "chenglin", now); err != nil {
		t.Fatalf("create host intervention: %v", err)
	}

	first := world.BuildResidentWorldView(profile, 10)
	if !strings.Contains(first.RenderedChat, "recent_host_interventions:") {
		t.Fatalf("expected intervention block in rendered chat")
	}
	if len(first.FreshDeliveredItems) != 1 {
		t.Fatalf("expected 1 fresh host intervention update, got %d", len(first.FreshDeliveredItems))
	}
	if !strings.Contains(first.FreshDeliveredItems[0], "host_intervention_update") {
		t.Fatalf("expected host_intervention_update marker, got %q", first.FreshDeliveredItems[0])
	}

	second := world.BuildResidentWorldView(profile, 10)
	if len(second.FreshDeliveredItems) != 0 {
		t.Fatalf("expected host intervention fresh update to be consumed once, got %#v", second.FreshDeliveredItems)
	}
}
