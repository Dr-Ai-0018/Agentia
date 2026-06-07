package newborn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-arena/internal/brokerstate"
	"ai-arena/internal/context"
	"ai-arena/internal/memory"
	"ai-arena/internal/openai"
	"ai-arena/internal/worldstate"
)

func TestBuildProfile(t *testing.T) {
	profile, err := BuildProfile("amber")
	if err != nil {
		t.Fatalf("build profile: %v", err)
	}
	if profile.Model != "gpt-5.5" {
		t.Fatalf("unexpected model: %s", profile.Model)
	}
}

func TestPreflightSpecBootstrap(t *testing.T) {
	spec := preflightSpec(ResidentProfile{Name: "amber", Model: "gpt-5.5"}, loopState{}, time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC))
	if spec.Usage.InputTokens != 1400 {
		t.Fatalf("unexpected bootstrap input: %d", spec.Usage.InputTokens)
	}
	if spec.Usage.OutputTokens != 350 {
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
	}, time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC))
	if spec.Usage.InputTokens < 1000 {
		t.Fatalf("expected inflated input tokens")
	}
	if spec.Usage.CachedTokens != 200 {
		t.Fatalf("unexpected cached tokens: %d", spec.Usage.CachedTokens)
	}
	if spec.Usage.OutputTokens < 300 {
		t.Fatalf("expected inflated output tokens")
	}
}

func TestFallbackAcceptance(t *testing.T) {
	got := fallbackAcceptance(nil, "broker_preflight_denied")
	if got == "" {
		t.Fatalf("expected fallback acceptance text")
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

func TestRepeatedWriteNoteSuppression(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "boot-notes.md")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	command := "cat > " + target + " <<'EOF'\nnew\nEOF"
	if !repeatedWriteNote(command) {
		t.Fatalf("expected repeated write note to be suppressed")
	}
}

func TestRenderRecentActions(t *testing.T) {
	lines := renderRecentActions([]RecentAction{
		{
			Round:       2,
			Action:      "write_note",
			Intent:      "baseline_note_capture",
			Reason:      "preserve baseline",
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
	if !strings.Contains(lines[0], "intent=baseline_note_capture") {
		t.Fatalf("expected intent marker in %q", lines[0])
	}
}

func TestClassifyCommandIntentDetectsBaselineCaptureInsideGuestExec(t *testing.T) {
	intent := classifyCommandIntent(AgentDecision{
		NextAction: "guest_exec",
		Command:    "cat > /root/arena-notes/boot-notes.md <<'EOF'\n- Hostname: onyx\n- Kernel: Linux\n- Disk: 12G\n- Memory: 2G\n- Debian trixie\nEOF",
	})
	if intent != "baseline_note_capture" {
		t.Fatalf("expected baseline_note_capture, got %q", intent)
	}
}

func TestRepeatedBaselineCaptureByRecentFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "boot-notes.md")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	command := "cat > " + target + " <<'EOF'\n- Hostname: onyx\n- Kernel: Linux\n- Disk: 12G\n- Memory: 2G\n- Debian trixie\nEOF"
	if !repeatedBaselineCapture(command) {
		t.Fatalf("expected repeated baseline capture to be suppressed")
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
	if !ok || next != SurfaceNetwork {
		t.Fatalf("expected next frontier network, got %q ok=%v", next, ok)
	}
}

func TestRenderExplorationFrontierIncludesCompletionFlag(t *testing.T) {
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
	if !strings.Contains(joined, "baseline_capture_complete=true") {
		t.Fatalf("expected completion flag in %q", joined)
	}
	if !strings.Contains(joined, "next_preferred_surface=services") {
		t.Fatalf("expected services as next frontier in %q", joined)
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
	if !ok || nextComfortable != SurfaceServices {
		t.Fatalf("expected comfortable frontier to prefer services, got %q ok=%v", nextComfortable, ok)
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
			AfterStatus: &brokerstate.ResidentStatus{
				SparkBalance: 4.125,
				Window6HCap:  12000,
				Window6HUsed: 4800,
				DayCap:       60000,
				DayUsed:      7200,
				WeekCap:      150000,
				WeekUsed:     11000,
				DebtAmount:   0,
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
		"spark_balance_before=4.5000",
		"spark_balance_after=4.1250",
		"window_6h_remaining=7200",
		"next_call_estimated_input_tokens=1150",
		"next_call_estimated_output_tokens=229",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("expected budget fact %q in %q", required, joined)
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
		"I confirmed root-level control inside my own VM.",
		"The machine identifies itself as Debian 13 (trixie).",
		"The machine name resolves as amber.",
		"There are already local notes and continuity files in the home directory.",
		"The network interface and default route are present inside the VM.",
		"Outbound IPv4, DNS resolution, and HTTPS reachability all worked in direct checks.",
		"Core guest services are alive, including incus-agent and systemd networking.",
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
		"I inspected basic resource state such as memory, disk, CPU, or uptime.",
		"I mapped part of the home directory and the local note surfaces.",
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
	if !strings.Contains(joined, "old system-written directive") {
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

	observation := exec.Execute(ResidentProfile{Name: "amber"}, AgentDecision{
		NextAction:    "memory_review",
		MemoryID:      "amber-short-legacy",
		MemoryAction:  "rewrite",
		MemorySummary: "I rewrote this into a cleaner carry-forward note.",
		MemoryReason:  "The previous version was too raw.",
		Reason:        "I want a useful short memory.",
	})
	if !strings.Contains(observation, "memory review applied:") {
		t.Fatalf("expected memory review observation, got %q", observation)
	}
	updated, ok, err := exec.memories.GetAbstractMemory("amber", "amber-short-legacy")
	if err != nil || !ok {
		t.Fatalf("get updated memory: ok=%v err=%v", ok, err)
	}
	if updated.Summary != "I rewrote this into a cleaner carry-forward note." {
		t.Fatalf("unexpected rewritten summary: %q", updated.Summary)
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
