package newborn

import (
	"fmt"
	"strings"
)

const (
	maxObservationHistoryChars = 2200
	maxObservationHistoryLines = 80
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
	if len(compacted) > maxObservationHistoryChars {
		compacted = compacted[:maxObservationHistoryChars]
		truncatedByChars = true
	}
	compacted = strings.TrimSpace(compacted)

	if truncatedByLines || truncatedByChars {
		suffix := fmt.Sprintf("\n[observation truncated for context reuse: original_lines=%d original_chars=%d kept_lines=%d kept_chars=%d]",
			len(strings.Split(trimmed, "\n")),
			len(trimmed),
			len(strings.Split(compacted, "\n")),
			len(compacted),
		)
		compacted += suffix
	}

	return compacted
}
