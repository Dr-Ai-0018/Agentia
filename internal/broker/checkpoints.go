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
	label = strings.ToLower(strings.TrimSpace(label))
	label = strings.ReplaceAll(label, " ", "-")
	if residentID == "" {
		residentID = "resident"
	}
	if label == "" {
		label = "snapshot"
	}
	return fmt.Sprintf("self-%s-%s", residentID, label)
}
