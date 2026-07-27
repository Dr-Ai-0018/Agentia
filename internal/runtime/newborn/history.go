package newborn

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxObservationHistoryChars = 1200
	maxObservationHistoryLines = 40
)

func compactObservationForHistory(observation string) string {
	trimmed := strings.TrimSpace(observation)
	if trimmed == "" {
		return trimmed
	}

	lines := strings.Split(trimmed, "\n")
	truncatedByLines := false
	if len(lines) > maxObservationHistoryLines {
		lines = lines[:maxObservationHistoryLines]
		truncatedByLines = true
	}

	compacted := strings.Join(lines, "\n")
	truncatedByChars := false
	if utf8.RuneCountInString(compacted) > maxObservationHistoryChars {
		compacted = truncateRunes(compacted, maxObservationHistoryChars)
		truncatedByChars = true
	}
	compacted = strings.TrimSpace(compacted)

	if truncatedByLines || truncatedByChars {
		suffix := fmt.Sprintf("\n[observation shortened: original_lines=%d original_chars=%d kept_lines=%d kept_chars=%d]",
			len(strings.Split(trimmed, "\n")),
			utf8.RuneCountInString(trimmed),
			len(strings.Split(compacted, "\n")),
			utf8.RuneCountInString(compacted),
		)
		compacted += suffix
	}

	return compacted
}

func truncateForModel(s string, limit int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if limit <= 0 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	if limit <= 3 {
		return truncateRunes(s, limit)
	}
	return strings.TrimSpace(truncateRunes(s, limit-3)) + "..."
}

func truncateRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

func validUTF8Prefix(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.ValidString(s[:maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

func validUTF8Suffix(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	start := len(s) - maxBytes
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}
