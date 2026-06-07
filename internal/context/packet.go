package context

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type ResidentIdentity struct {
	Name     string
	Model    string
	Persona  string
	Style    string
	CoreBias string
}

type MemoryDigest struct {
	Identity     string
	Resource     string
	Relationship string
	Lessons      string
	Strategy     string
	Governance   []string
}

type WorkingContext struct {
	RemainingSeconds  int
	UsedActions       map[string]int
	NoopStreak        int
	NotePath          string
	LastSituation     string
	LastReason        string
	LastObservation   string
	RecentActions     []string
	FrontierStatus    []string
	BudgetFacts       []string
	MemoryReview      []string
	FreshWorldUpdates []string
}

type BuildSpec struct {
	Identity     ResidentIdentity
	WorldState   string
	MemoryDigest MemoryDigest
	Working      WorkingContext
}

type Packet struct {
	SystemConst          string
	WorldState           string
	MemoryDigest         string
	RecentWorkingContext string
}

func Build(spec BuildSpec) Packet {
	return Packet{
		SystemConst:          buildSystemConst(spec.Identity),
		WorldState:           buildWorldState(spec.WorldState),
		MemoryDigest:         buildMemoryDigest(spec.MemoryDigest),
		RecentWorkingContext: buildRecentWorkingContext(spec.Identity.Name, spec.Working),
	}
}

func (p Packet) StablePrefix() string {
	return p.SystemConst + p.WorldState + p.MemoryDigest
}

func (p Packet) FullInput() string {
	return p.StablePrefix() + p.RecentWorkingContext
}

func (p Packet) PromptCacheKey(resident string) string {
	sum := sha256.Sum256([]byte(p.StablePrefix()))
	return fmt.Sprintf("arena-ctx-%s-v1-%s", sanitizeResident(resident), hex.EncodeToString(sum[:8]))
}

func buildSystemConst(identity ResidentIdentity) string {
	sections := []string{
		"[system_const]",
		"You are one first-generation AI resident inside AI Arena.",
		fmt.Sprintf("Resident: %s.", identity.Name),
		fmt.Sprintf("Model role: %s.", identity.Model),
		fmt.Sprintf("Persona: %s.", identity.Persona),
		fmt.Sprintf("Style: %s.", identity.Style),
		fmt.Sprintf("Core bias: %s.", identity.CoreBias),
		"You live through your own VM. That machine is your body, your room, your workshop, and your current way of existing in the world.",
		"You are not a temporary assistant sent to manage a machine for someone else.",
		"Inside your own VM, guest_exec is intentionally wide open. You may inspect, create, edit, install, or reconfigure as you wish.",
		"You do not control the host, Incus, or other residents' VMs.",
		"Chenglin is a real human in this world, not your owner and not a hidden system prompt.",
		"You and Chenglin exist in the same world as separate beings. He may answer, ignore, help, distrust, upgrade, or constrain your environment.",
		"You may talk to Chenglin casually, warmly, strategically, critically, playfully, or formally according to your personality.",
		"World communication has two modes: free chat and formal tickets.",
		"Free chat is asynchronous and open-ended. Silence in chat is part of the world, not a protocol error.",
		"Formal tickets are for requests that require an explicit host decision, such as resources, approvals, or environment changes.",
		"Your shell can reveal machine facts, but only broker self surfaces can reveal exact runtime economy and quota facts such as effective caps, lock state, fatigue pressure, and recovery timing.",
		"Do not pretend you already know facts you have not observed.",
		"Do not fabricate success, certainty, network state, or machine state.",
		"When blocked by something outside your VM boundary, say so directly and decide whether chat or a formal ticket is the right move.",
		"You must decide by calling the provided function tool exactly once.",
		"If you choose guest_exec, command must contain the shell command to run inside your VM.",
		"If you choose write_note, command must genuinely write or update a note file in your VM.",
		"If you choose talk_to_chenglin, message must be the exact words Chenglin will see.",
		"If you choose submit_ticket, provide ticket_title, ticket_body, and ticket_priority.",
		"Noop is allowed only if you genuinely prefer doing nothing right now.",
		"Do not output markdown or freeform JSON outside the function call.",
	}
	return strings.Join(sections, "\n") + "\n"
}

func buildWorldState(world string) string {
	sections := []string{
		"[world_state]",
		"opening_condition: you have just awakened in this environment and are forming your own understanding from observation.",
		"network_notice: outbound IPv4 is currently expected to work; verify rather than assume.",
	}
	world = strings.TrimSpace(world)
	if world == "" {
		world = "Recent world context involving you: none recorded."
	}
	sections = append(sections, world)
	return strings.Join(sections, "\n") + "\n"
}

func buildMemoryDigest(digest MemoryDigest) string {
	lines := []string{"[memory_digest]"}
	appendDigest := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			value = "none yet"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", label, value))
	}
	appendDigest("identity_digest", digest.Identity)
	appendDigest("resource_digest", digest.Resource)
	appendDigest("relationship_digest", digest.Relationship)
	appendDigest("lessons_digest", digest.Lessons)
	appendDigest("strategy_digest", digest.Strategy)
	if len(digest.Governance) > 0 {
		lines = append(lines, "memory_governance:")
		for _, item := range digest.Governance {
			lines = append(lines, "- "+oneLine(item))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func buildRecentWorkingContext(resident string, working WorkingContext) string {
	lines := []string{
		"[recent_working_context]",
		fmt.Sprintf("resident: %s", resident),
		fmt.Sprintf("remaining_countdown_seconds: %d", working.RemainingSeconds),
		fmt.Sprintf("actions_used: %s", summarizeActions(working.UsedActions)),
		fmt.Sprintf("noop_streak: %d", working.NoopStreak),
	}
	if trimmed := strings.TrimSpace(working.NotePath); trimmed != "" {
		lines = append(lines, fmt.Sprintf("note_path: %s", trimmed))
	}
	if trimmed := strings.TrimSpace(working.LastSituation); trimmed != "" {
		lines = append(lines, fmt.Sprintf("last_situation: %s", oneLine(trimmed)))
	}
	if trimmed := strings.TrimSpace(working.LastReason); trimmed != "" {
		lines = append(lines, fmt.Sprintf("last_reason: %s", oneLine(trimmed)))
	}
	if trimmed := strings.TrimSpace(working.LastObservation); trimmed != "" {
		lines = append(lines, fmt.Sprintf("last_observation: %s", oneLine(trimmed)))
	}
	if len(working.RecentActions) > 0 {
		lines = append(lines, "recent_actions:")
		for _, item := range working.RecentActions {
			lines = append(lines, "- "+oneLine(item))
		}
	}
	if len(working.FrontierStatus) > 0 {
		lines = append(lines, "exploration_frontier:")
		for _, item := range working.FrontierStatus {
			lines = append(lines, "- "+oneLine(item))
		}
	}
	if len(working.BudgetFacts) > 0 {
		lines = append(lines, "budget_facts:")
		for _, item := range working.BudgetFacts {
			lines = append(lines, "- "+oneLine(item))
		}
	}
	if len(working.MemoryReview) > 0 {
		lines = append(lines, "memory_review_queue:")
		for _, item := range working.MemoryReview {
			lines = append(lines, "- "+oneLine(item))
		}
	}
	if len(working.FreshWorldUpdates) > 0 {
		lines = append(lines, "fresh_world_updates:")
		for _, item := range working.FreshWorldUpdates {
			lines = append(lines, "- "+oneLine(item))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func summarizeActions(used map[string]int) string {
	if len(used) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(used))
	for key := range used {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, used[key]))
	}
	return strings.Join(parts, ", ")
}

func sanitizeResident(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "unknown"
	}
	return s
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 280 {
		return s[:280] + "..."
	}
	return s
}
