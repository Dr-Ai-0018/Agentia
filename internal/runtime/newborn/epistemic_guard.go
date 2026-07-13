package newborn

import "strings"

type EpistemicGuardViolation struct {
	Term   string
	Sample string
}

var compactionReservedTerms = []string{
	"context window",
	"context length",
	"token budget",
	"token limit",
	"compaction",
	"summarization",
	"summary",
	"another llm",
	"you are an ai",
	"you are a model",
	"runtime",
	"harness",
	"session",
	"operator",
	"administrator",
	"admin gate",
	"test allowance",
	"calibration",
	"上下文窗口",
	"上下文长度",
	"token",
	"令牌预算",
	"压缩",
	"摘要",
	"总结调用",
	"另一个模型",
	"我是 ai",
	"我是一个模型",
	"运行时",
	"会话",
	"管理员",
	"操作员",
	"测试额度",
	"校准",
}

func checkCompactionEpistemicGuard(text string) *EpistemicGuardViolation {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return &EpistemicGuardViolation{Term: "empty_output", Sample: ""}
	}
	for _, term := range compactionReservedTerms {
		needle := strings.ToLower(term)
		if strings.Contains(normalized, needle) {
			return &EpistemicGuardViolation{
				Term:   term,
				Sample: redactedGuardSample(text, term),
			}
		}
	}
	return nil
}

func redactedGuardSample(text, term string) string {
	if text == "" {
		return ""
	}
	_ = term
	return "(输出含保留词，已脱敏丢弃)"
}
