package broker

import (
	"fmt"
	"strings"
	"time"
)

func BaselineSnapshotName() string {
	return "clean-base"
}

func HostCheckpointName(residentID string, now time.Time) string {
	residentID = strings.ToLower(strings.TrimSpace(residentID))
	if residentID == "" {
		residentID = "resident"
	}
	return fmt.Sprintf("checkpoint-%s-%s", residentID, now.UTC().Format("20060102T150405Z"))
}

func ResidentSelfSnapshotName(residentID, label string) string {
	residentID = strings.ToLower(strings.TrimSpace(residentID))
	label = sanitizeSnapshotLabel(label)
	if residentID == "" {
		residentID = "resident"
	}
	if label == "" {
		label = "snapshot"
	}
	return fmt.Sprintf("self-%s-%s", residentID, label)
}

func NormalizeResidentSelfSnapshotName(residentID, snapshotName string) string {
	raw := strings.TrimSpace(snapshotName)
	name := strings.ToLower(raw)
	prefix := "self-" + strings.ToLower(strings.TrimSpace(residentID)) + "-"
	if strings.HasPrefix(name, prefix) {
		return raw
	}
	if name == BaselineSnapshotName() || strings.HasPrefix(name, "checkpoint-") {
		return raw
	}
	return ResidentSelfSnapshotName(residentID, name)
}

func sanitizeSnapshotLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	var b strings.Builder
	lastDash := false
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
