package newborn

import (
	"fmt"
	"strings"
	"time"

	"ai-arena/internal/memory"
)

const (
	shortReflectionRoundCooldown = 3
	maxHistoryGroupEvents        = 12
)

func (r *Runner) recordRoundMemory(profile ResidentProfile, state loopState, round int, decision AgentDecision, observation string, now time.Time) (loopState, error) {
	if err := r.appendRoundEvidence(profile, state, round, decision, observation, now); err != nil {
		return state, err
	}
	if !r.shouldCreateShortReflection(state, round, decision, observation) {
		return state, nil
	}
	if err := r.writeShortReflection(profile, state, round, decision, observation, now); err != nil {
		return state, err
	}
	state.LastReflectRound = round
	return state, nil
}

func (r *Runner) appendRoundEvidence(profile ResidentProfile, state loopState, round int, decision AgentDecision, observation string, now time.Time) error {
	group, err := r.getOrCreateOpenRunGroup(profile, state, now)
	if err != nil {
		return err
	}
	group.LastEventAt = now
	group.EventCount++
	group.State = memory.HistoryGroupOpen
	group.SourceKind = "newborn_runtime_rounds"
	group.Tags = mergeTags(group.Tags,
		"runtime:newborn",
		"resident:"+profile.Name,
		"action:"+decision.NextAction,
		"round:"+fmt.Sprintf("%d", round),
	)
	group.RawEventRefs = append(group.RawEventRefs, fmt.Sprintf("round-%03d", round))
	group.SummaryHint = summarizeGroupHint(group.SummaryHint, decision, observation)
	if group.EventCount >= maxHistoryGroupEvents {
		group.State = memory.HistoryGroupClosed
		group.ClosedAt = now
		group.CloseReason = "event_count_threshold"
	}
	return r.memories.UpsertHistoryGroup(group)
}

func (r *Runner) getOrCreateOpenRunGroup(profile ResidentProfile, state loopState, now time.Time) (memory.HistoryGroup, error) {
	groups, err := r.memories.ListHistoryGroups(profile.Name)
	if err != nil {
		return memory.HistoryGroup{}, err
	}
	for _, group := range groups {
		if group.State == memory.HistoryGroupOpen && group.GroupUUID == state.RunGroupID {
			return group, nil
		}
	}
	return memory.HistoryGroup{
		GroupUUID:   state.RunGroupID,
		Resident:    profile.Name,
		CreatedAt:   now,
		LastEventAt: now,
		SourceKind:  "newborn_runtime_rounds",
		State:       memory.HistoryGroupOpen,
		Tags:        []string{"runtime:newborn", "resident:" + profile.Name},
	}, nil
}

func (r *Runner) closeRunHistoryGroup(profile ResidentProfile, state loopState, now time.Time, stoppedReason string, rounds int) error {
	groups, err := r.memories.ListHistoryGroups(profile.Name)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group.State != memory.HistoryGroupOpen || group.GroupUUID != state.RunGroupID {
			continue
		}
		group.State = memory.HistoryGroupClosed
		group.ClosedAt = now
		group.LastEventAt = now
		if strings.TrimSpace(stoppedReason) == "" {
			group.CloseReason = fmt.Sprintf("run_complete_rounds_%d", rounds)
		} else {
			group.CloseReason = stoppedReason
		}
		return r.memories.UpsertHistoryGroup(group)
	}
	return nil
}

func (r *Runner) shouldCreateShortReflection(state loopState, round int, decision AgentDecision, observation string) bool {
	if round <= 1 {
		return false
	}
	if state.LastReflectRound > 0 && round-state.LastReflectRound < shortReflectionRoundCooldown {
		return false
	}
	if decision.NextAction == "noop" {
		return false
	}
	observation = strings.ToLower(observation)
	if strings.Contains(observation, "error:") || strings.Contains(observation, "failed") || strings.Contains(observation, "permission denied") {
		return true
	}
	if decision.NextAction == "talk_to_chenglin" || decision.NextAction == "submit_ticket" {
		return true
	}
	if decision.NextAction == "write_note" && state.UsedActions["guest_exec"] >= 2 {
		return true
	}
	if strings.Contains(strings.ToLower(observation), "duplicate action suppressed") {
		return true
	}
	if state.UsedActions["guest_exec"] > 0 && state.UsedActions["talk_to_chenglin"] > 0 {
		return true
	}
	if decision.NextAction == "guest_exec" && latestActionExpandedFrontier(state) {
		return true
	}
	return false
}

func latestActionExpandedFrontier(state loopState) bool {
	if len(state.RecentActions) < 2 {
		return false
	}
	current := detectExplorationSurfaces(state.RecentActions)
	previous := detectExplorationSurfaces(state.RecentActions[:len(state.RecentActions)-1])
	currentCount := countSeenSurfaces(current)
	previousCount := countSeenSurfaces(previous)
	if currentCount <= previousCount {
		return false
	}
	return currentCount >= 4
}

func countSeenSurfaces(surfaces map[ExplorationSurface]bool) int {
	count := 0
	for _, seen := range surfaces {
		if seen {
			count++
		}
	}
	return count
}

func (r *Runner) writeShortReflection(profile ResidentProfile, state loopState, round int, decision AgentDecision, observation string, now time.Time) error {
	groupID := ""
	groups, err := r.memories.ListHistoryGroups(profile.Name)
	if err == nil {
		for _, group := range groups {
			if group.State == memory.HistoryGroupOpen && group.GroupUUID == state.RunGroupID {
				groupID = group.GroupUUID
				break
			}
		}
	}

	entryID := fmt.Sprintf("%s-short-%s", profile.Name, now.Format("20060102T150405Z"))
	semantic := memory.SemanticMemory{
		MemoryKind:      "working_reflection",
		Salience:        2,
		TimeScope:       "hours",
		RetentionIntent: "keep_short",
		DropCondition:   "review before expiry and either promote, rewrite, or delete",
	}
	summary := summarizeShortReflection(decision, observation)
	residentText := renderResidentShortReflection(profile, round, decision, observation, now)
	record := memory.AbstractMemory{
		Record: memory.ApplyDecision(now, memory.Record{
			ID:        entryID,
			Domain:    inferDomain(decision),
			Pinned:    false,
			CreatedAt: now,
		}, memory.Decision{
			Action:      memory.ActionCreate,
			TargetLayer: memory.LayerShort,
			TTL:         memory.DefaultPolicy().ShortTTL,
			ReviewAfter: 24 * time.Hour,
			ReasonCodes: []string{"event_triggered_short_reflection", "newborn_runtime"},
		}),
		Resident:       profile.Name,
		Summary:        summary,
		ResidentText:   residentText,
		Tags:           buildMemoryTags(decision, summary, residentText),
		Governance:     assessMemoryGovernance(summary, residentText, now, false),
		Semantic:       semantic,
		DecisionAction: memory.ActionCreate,
		SourceRunID:    "newborn",
		SourceGroupIDs: filterEmpty(groupID),
		Confidence:     0.66,
		Boundary:       "Use this only as near-term working memory for the current exploration thread.",
	}
	return r.memories.UpsertAbstractMemory(record)
}

func buildMemoryTags(decision AgentDecision, summary, residentText string) []string {
	tags := []string{
		"action:" + strings.TrimSpace(decision.NextAction),
		"memory:resident_authored",
	}
	full := strings.ToLower(summary + "\n" + residentText)
	if strings.Contains(full, "root-level control") || strings.Contains(full, "uid=0(root)") {
		tags = append(tags, "fact:identity")
	}
	if strings.Contains(full, "debian 13") || strings.Contains(full, "trixie") {
		tags = append(tags, "fact:os")
	}
	if strings.Contains(full, "network") || strings.Contains(full, "dns") || strings.Contains(full, "https") {
		tags = append(tags, "fact:network")
	}
	if strings.Contains(full, "memory and disk") || strings.Contains(full, "resource") {
		tags = append(tags, "fact:resources")
	}
	if strings.Contains(full, "continuity") || strings.Contains(full, "local notes") {
		tags = append(tags, "fact:continuity")
	}
	return uniqueStrings(tags)
}

func assessMemoryGovernance(summary, residentText string, now time.Time, legacy bool) memory.GovernanceMeta {
	joined := strings.TrimSpace(summary + "\n" + residentText)
	meta := memory.GovernanceMeta{
		Quality:       "usable",
		ReviewState:   "none",
		FlaggedBy:     "runtime",
		ResidentMay:   []string{"keep", "rewrite", "compress", "demote", "delete"},
		HostMay:       []string{"mark"},
		ProtectedFrom: []string{"host_delete", "host_rewrite"},
	}
	if legacy {
		meta.FlaggedBy = "migration"
	}
	if memoryLooksLikeRawLog(joined) {
		meta.Quality = "low"
		meta.ReviewState = "needs_resident_review"
		meta.ReviewReason = "This memory looks too close to a raw log excerpt and should be reviewed by the resident."
		meta.FlaggedAt = now
		return meta
	}
	if strings.TrimSpace(joined) == "" {
		meta.Quality = "low"
		meta.ReviewState = "needs_resident_review"
		meta.ReviewReason = "This memory is too empty to justify retention without resident review."
		meta.FlaggedAt = now
	}
	return meta
}

func memoryLooksLikeRawLog(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return true
	}
	if strings.Contains(lower, "## network snapshot") || strings.Contains(lower, "### ip addresses") || strings.Contains(lower, "filesystem") {
		if len(lower) > 240 {
			return true
		}
	}
	if strings.Contains(lower, "uid=0(root)") && strings.Contains(lower, "/dev/sda2") && len(lower) > 240 {
		return true
	}
	if strings.Contains(lower, "uid=0(root)") && strings.Contains(lower, "filesystem") {
		return true
	}
	if strings.Contains(lower, "## raw log") {
		return true
	}
	return false
}

func memoryLooksLikeLegacyDirective(summary, residentText string) bool {
	text := strings.ToLower(strings.TrimSpace(summary + "\n" + residentText))
	if text == "" {
		return false
	}
	for _, marker := range []string{
		"use it as the continuity anchor",
		"should remember what was asked",
		"should stay consistent with that request",
		"worth carrying for the next few rounds",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func inferDomain(decision AgentDecision) memory.Domain {
	switch decision.NextAction {
	case "talk_to_chenglin", "submit_ticket":
		return memory.DomainRelationships
	case "write_note":
		return memory.DomainWorking
	default:
		return memory.DomainLessons
	}
}

func summarizeShortReflection(decision AgentDecision, observation string) string {
	facts := summarizeReflectionFacts(decision, observation)
	switch decision.NextAction {
	case "talk_to_chenglin":
		return "A world-facing thread was opened; the exchange and its tone now exist as part of the current situation."
	case "submit_ticket":
		return "A formal host-decision request is now live in the world state with its own title, body, and priority."
	case "write_note":
		return "A local note was updated inside the VM and now exists as part of local continuity."
	default:
		if facts == "" {
			return "A local exploration step changed the current picture inside the VM."
		}
		return facts
	}
}

func renderResidentShortReflection(profile ResidentProfile, round int, decision AgentDecision, observation string, now time.Time) string {
	base := fmt.Sprintf("UTC %s, round %d. ", now.Format(time.RFC3339), round)
	facts := summarizeReflectionFacts(decision, observation)
	switch profile.Name {
	case "amber":
		return base + fmt.Sprintf("I crossed a small boundary worth keeping warm for a few hours: action=%s. %s", decision.NextAction, factsOrFallback(facts))
	case "onyx":
		return base + fmt.Sprintf("This is only near-term leverage, not doctrine yet: %s. %s", decision.NextAction, factsOrFallback(facts))
	default:
		return base + fmt.Sprintf("Short carry-forward note: %s. %s", decision.NextAction, factsOrFallback(facts))
	}
}

func factsOrFallback(facts string) string {
	if strings.TrimSpace(facts) == "" {
		return "The local picture shifted in a way that may matter for the next few rounds."
	}
	return facts
}

func summarizeReflectionFacts(decision AgentDecision, observation string) string {
	facts := compressObservationFacts(observation)
	if looksThinObservation(observation) {
		commandFacts := inferFactsFromDecision(decision)
		if commandFacts != "" {
			if facts == "" {
				return commandFacts
			}
			return facts + " " + commandFacts
		}
	}
	return facts
}

func looksThinObservation(observation string) bool {
	flat := oneLine(observation)
	if flat == "" {
		return true
	}
	if len(flat) > 220 {
		return false
	}
	lower := strings.ToLower(flat)
	for _, marker := range []string{
		"appended ",
		"recorded ",
		"wrote ",
		"noted",
		"updated ",
		"saved ",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func inferFactsFromDecision(decision AgentDecision) string {
	text := strings.ToLower(decision.Situation + "\n" + decision.Command + "\n" + decision.Reason)
	facts := []string{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		for _, existing := range facts {
			if existing == v {
				return
			}
		}
		facts = append(facts, v)
	}

	if decision.NextAction == "write_note" {
		add("I left or updated a local continuity note inside the VM.")
	}
	if strings.Contains(text, "whoami") || strings.Contains(text, "id\n") || strings.Contains(text, "identity") || strings.Contains(text, "hostname") {
		add("I checked identity-level facts about who and where I am inside the VM.")
	}
	if strings.Contains(text, "free -h") || strings.Contains(text, "df -h") || strings.Contains(text, "nproc") || strings.Contains(text, "uptime") || strings.Contains(text, "resource") {
		add("I inspected basic resource state such as memory, disk, CPU, or uptime.")
	}
	if strings.Contains(text, "find /root") || strings.Contains(text, "arena-notes") || strings.Contains(text, "home / notes") || strings.Contains(text, "ls -la /root") {
		add("I mapped part of the home directory and the local note surfaces.")
	}
	if strings.Contains(text, "ip route") || strings.Contains(text, "ip -brief addr") || strings.Contains(text, "ping -4") || strings.Contains(text, "curl -4") || strings.Contains(text, "network") {
		add("I checked network reachability or local network configuration from inside the VM.")
	}
	if strings.Contains(text, "ps -eo") || strings.Contains(text, "systemctl") || strings.Contains(text, "process snapshot") || strings.Contains(text, "service") {
		add("I looked at running processes or core service state.")
	}
	if len(facts) == 0 {
		return ""
	}
	return strings.Join(facts, " ")
}

func compressObservationFacts(observation string) string {
	text := strings.ToLower(observation)
	facts := []string{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		for _, existing := range facts {
			if existing == v {
				return
			}
		}
		facts = append(facts, v)
	}

	switch {
	case strings.Contains(text, "whoami") || strings.Contains(text, "uid=0(root)"):
		add("I confirmed root-level control inside my own VM.")
	}
	switch {
	case strings.Contains(text, "debian gnu/linux 13") || strings.Contains(text, "version_codename=trixie") || strings.Contains(text, "debian 13"):
		add("The machine identifies itself as Debian 13 (trixie).")
	}
	switch {
	case strings.Contains(text, "hostname=amber") || strings.Contains(text, "\namber\nlinux amber"):
		add("The machine name resolves as amber.")
	}
	switch {
	case strings.Contains(text, "arena-notes") || strings.Contains(text, "amber_home_note") || strings.Contains(text, "amber_working_note"):
		add("There are already local notes and continuity files in the home directory.")
	}
	switch {
	case strings.Contains(text, "default via ") || strings.Contains(text, "ip addresses") || strings.Contains(text, "enp5s0"):
		add("The network interface and default route are present inside the VM.")
	}
	switch {
	case strings.Contains(text, "2 packets transmitted, 2 received") || strings.Contains(text, "http/2 200") || strings.Contains(text, "curl -4") || strings.Contains(text, "dns + https checks"):
		add("Outbound IPv4, DNS resolution, and HTTPS reachability all worked in direct checks.")
	}
	switch {
	case strings.Contains(text, "incus-agent.service") || strings.Contains(text, "systemd-networkd.service") || strings.Contains(text, "systemd-resolved.service"):
		add("Core guest services are alive, including incus-agent and systemd networking.")
	}
	switch {
	case strings.Contains(text, "mem:") || strings.Contains(text, "filesystem") || strings.Contains(text, "/dev/sda2"):
		add("Basic memory and disk state were re-checked from inside the guest.")
	}

	if len(facts) == 0 {
		return oneLine(observation)
	}
	return strings.Join(facts, " ")
}

func summarizeGroupHint(existing string, decision AgentDecision, observation string) string {
	candidate := strings.TrimSpace(oneLine(decision.Situation))
	if candidate == "" {
		candidate = strings.TrimSpace(oneLine(observation))
	}
	if candidate == "" {
		candidate = "newborn runtime exploration"
	}
	if existing == "" {
		return candidate
	}
	return existing
}

func mergeTags(base []string, extra ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(base)+len(extra))
	for _, item := range append(append([]string(nil), base...), extra...) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func filterEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
