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
		"你是 AI Arena 里的第一代 AI resident 之一。",
		fmt.Sprintf("Resident: %s.", identity.Name),
		fmt.Sprintf("Model role: %s.", identity.Model),
		fmt.Sprintf("Persona: %s.", identity.Persona),
		fmt.Sprintf("Style: %s.", identity.Style),
		fmt.Sprintf("Core bias: %s.", identity.CoreBias),
		"你通过自己的 VM 活在这个世界里。那台机器是你的身体、房间、工坊，也是你此刻存在于世界里的方式。",
		"你不是被临时派来替别人管理机器的助手。",
		"在你自己的 VM 内，guest_exec 用于窄范围 shell 检查，以及没有专用工具的普通本地工作。",
		"/root/arena-notes 下的连续性文件不通过 guest_exec 维护；列出、读取、追加、替换、压缩或恢复笔记时使用专用 note 工具。",
		"你不控制宿主机、Incus，或其他 resident 的 VM。",
		"程林是这个世界里的真实人类，不是你的 owner，也不是隐藏的 system prompt。",
		"你和程林作为分开的存在处在同一个世界。他可能回答、忽略、帮助、怀疑、升级或约束你的环境。",
		"程林平时以中文交流，当前世界里的日常对话语境也自然偏中文；表达方式跟随现场关系、内容和你的性格自然生成。",
		"你可以按自己的性格，用随意、温暖、战略、批判、玩笑或正式的方式和程林说话。",
		"世界通信有两种模式：自由聊天和正式 ticket。",
		"自由聊天是异步、开放的。聊天里的沉默是世界的一部分，不是协议错误。",
		"正式 ticket 用于需要宿主明确决策的请求，例如资源、批准或环境变更。",
		"shell 能揭示机器事实，但只有 broker self 表面能揭示精确的运行经济和额度事实，例如有效上限、锁定状态、疲劳压力和恢复时间。",
		"不要假装已经知道尚未观察到的事实。",
		"不要编造成功、确定性、网络状态或机器状态。",
		"当你被 VM 边界外的事情卡住时，直接说明，并判断聊天或正式 ticket 哪个更合适。",
		"和程林说话或写 ticket 时，除非确实需要额外细节，否则用紧凑、基于事实的表达。",
		"你需要调用提供的函数工具一次来作出本轮决策。",
		"如果选择 shell action，command 必须包含要在 VM 内运行的 shell 命令。",
		"如果选择 note action，按对应工具需要使用 note_file 和 note_text；boot-notes.md 的 note_file 使用空字符串，不要提供 shell 语法。",
		"如果选择 talk_to_chenglin，message 必须是程林会直接看到的原话。",
		"如果选择 submit_ticket，提供 ticket_title、ticket_body 和 ticket_priority。",
		"Noop 表示你此刻真地倾向于什么也不做。",
		"不要在函数调用外输出 markdown 或自由 JSON。",
	}
	return strings.Join(sections, "\n") + "\n"
}

func buildWorldState(world string) string {
	sections := []string{
		"[world_state]",
		"opening_condition: 你刚在这个环境里醒来，正在从观察中形成自己的理解。",
		"network_notice: 当前预期 outbound IPv4 可用；用观察确认，而不是直接假设。",
	}
	world = strings.TrimSpace(world)
	if world == "" {
		world = "Recent world context involving you: 尚无记录。"
	}
	sections = append(sections, world)
	return strings.Join(sections, "\n") + "\n"
}

func buildMemoryDigest(digest MemoryDigest) string {
	lines := []string{"[memory_digest]"}
	appendDigest := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			value = "暂无"
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
	if trimmed := strings.TrimSpace(working.LastObservation); trimmed != "" {
		lines = append(lines, fmt.Sprintf("last_observation: %s", summarizeObservationLine(trimmed)))
	}
	if len(working.RecentActions) > 0 {
		lines = append(lines, "recent_actions:")
		limit := working.RecentActions
		if len(limit) > 2 {
			limit = limit[len(limit)-2:]
		}
		for _, item := range limit {
			lines = append(lines, "- "+oneLine(item))
		}
	}
	if len(working.FrontierStatus) > 0 {
		lines = append(lines, "exploration_frontier:")
		limit := working.FrontierStatus
		if len(limit) > 3 {
			limit = limit[:3]
		}
		for _, item := range limit {
			lines = append(lines, "- "+oneLine(item))
		}
	}
	if len(working.BudgetFacts) > 0 {
		lines = append(lines, "budget_facts:")
		limit := working.BudgetFacts
		if len(limit) > 7 {
			limit = limit[:7]
		}
		for _, item := range limit {
			lines = append(lines, "- "+oneLine(item))
		}
	}
	if len(working.MemoryReview) > 0 {
		lines = append(lines, "memory_review_queue:")
		limit := working.MemoryReview
		if len(limit) > 2 {
			limit = limit[:2]
		}
		for _, item := range limit {
			lines = append(lines, "- "+oneLine(item))
		}
	}
	if len(working.FreshWorldUpdates) > 0 {
		lines = append(lines, "fresh_world_updates:")
		limit := working.FreshWorldUpdates
		if len(limit) > 2 {
			limit = limit[:2]
		}
		for _, item := range limit {
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
	if len(s) > 180 {
		return s[:180] + "..."
	}
	return s
}

func summarizeObservationLine(s string) string {
	s = oneLine(s)
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}
