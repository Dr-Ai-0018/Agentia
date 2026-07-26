package newborn

import (
	"strings"
	"unicode"
)

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
	"上下文",
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
		if containsReservedTerm(normalized, strings.ToLower(term)) {
			return &EpistemicGuardViolation{
				Term:   term,
				Sample: redactedGuardSample(text, term),
			}
		}
	}
	return nil
}

func containsReservedTerm(text, term string) bool {
	if term == "" {
		return false
	}
	if !asciiAlphaNumOnlyWithSpaces(term) {
		return strings.Contains(text, term)
	}
	start := 0
	for {
		idx := strings.Index(text[start:], term)
		if idx < 0 {
			return false
		}
		idx += start
		beforeOK := idx == 0 || !isASCIIAlphaNum(rune(text[idx-1]))
		afterIndex := idx + len(term)
		afterOK := afterIndex >= len(text) || !isASCIIAlphaNum(rune(text[afterIndex]))
		if beforeOK && afterOK {
			return true
		}
		start = idx + len(term)
	}
}

func asciiAlphaNumOnlyWithSpaces(s string) bool {
	hasAlphaNum := false
	for _, r := range s {
		switch {
		case r == ' ':
			continue
		case isASCIIAlphaNum(r):
			hasAlphaNum = true
		default:
			return false
		}
	}
	return hasAlphaNum
}

func isASCIIAlphaNum(r rune) bool {
	return r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
}

func redactedGuardSample(text, term string) string {
	if text == "" {
		return ""
	}
	_ = term
	return "(输出含保留词，已脱敏丢弃)"
}
