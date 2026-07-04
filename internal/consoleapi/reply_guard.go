package consoleapi

import (
	"fmt"
	"regexp"
	"strings"
)

var forbiddenReplyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bdashboard\b`),
	regexp.MustCompile(`仪表盘`),
	regexp.MustCompile(`(?i)\btelemetry\b`),
	regexp.MustCompile(`(?i)\brun\s*id\b`),
	regexp.MustCompile(`(?i)\borchestrator-\d{8}T`),
	regexp.MustCompile(`(?i)\btoken\b`),
	regexp.MustCompile(`令牌`),
	regexp.MustCompile(`(?i)\bcache\b`),
	regexp.MustCompile(`缓存`),
	regexp.MustCompile(`(?i)\binternal\s*usd\b`),
	regexp.MustCompile(`(?i)\busd\b`),
	regexp.MustCompile(`(?i)\$\s*\d`),
	regexp.MustCompile(`(?i)\bbilling\b`),
	regexp.MustCompile(`(?i)\bspend\b`),
	regexp.MustCompile(`(?i)\boperator\b`),
	regexp.MustCompile(`(?i)\badmin\b`),
	regexp.MustCompile(`管理员`),
	regexp.MustCompile(`后台`),
	regexp.MustCompile(`(?i)\bquota\b`),
	regexp.MustCompile(`(?i)\bbudget\b`),
	regexp.MustCompile(`(?i)\bspark\b`),
	regexp.MustCompile(`(?i)\bphase\b`),
	regexp.MustCompile(`(?i)\bround\b`),
	regexp.MustCompile(`(?i)\bintervention\b`),
	regexp.MustCompile(`(?i)\bmaintenance\b`),
	regexp.MustCompile(`(?i)\bP[0-2]\b`),
	regexp.MustCompile(`预算`),
	regexp.MustCompile(`审计`),
	regexp.MustCompile(`运维`),
	regexp.MustCompile(`维护`),
	regexp.MustCompile(`内部额度`),
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
