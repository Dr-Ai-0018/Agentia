package consoleapi

import (
	"fmt"
	"regexp"
	"strings"
)

var forbiddenReplyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bdashboard\b`),
	regexp.MustCompile(`(?i)\btelemetry\b`),
	regexp.MustCompile(`(?i)\brun\s*id\b`),
	regexp.MustCompile(`(?i)\btoken\b`),
	regexp.MustCompile(`(?i)\bcache\b`),
	regexp.MustCompile(`(?i)\binternal\s*usd\b`),
	regexp.MustCompile(`(?i)\boperator\b`),
	regexp.MustCompile(`(?i)\badmin\b`),
	regexp.MustCompile(`管理员`),
	regexp.MustCompile(`后台`),
	regexp.MustCompile(`隐藏测试`),
	regexp.MustCompile(`测试规则`),
	regexp.MustCompile(`验收`),
}

func validateWorldReply(body string, boundaryAck bool) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("reply body is required")
	}
	if !boundaryAck {
		return fmt.Errorf("boundary_ack must be true")
	}
	for _, pattern := range forbiddenReplyPatterns {
		if pattern.MatchString(body) {
			return fmt.Errorf("reply body contains private boundary term: %s", pattern.String())
		}
	}
	return nil
}
