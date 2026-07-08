package brokerstate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type QuotaEventKind string

const (
	QuotaEventWorkCall         QuotaEventKind = "work_call"
	QuotaEventAcceptanceCall   QuotaEventKind = "acceptance_call"
	QuotaEventFinalNotice      QuotaEventKind = "final_notice"
	QuotaEventTestAllowance    QuotaEventKind = "test_allowance"
	QuotaEventManualAdjustment QuotaEventKind = "manual_adjustment"
)

type QuotaEvent struct {
	ID         string                 `json:"id"`
	ResidentID string                 `json:"resident_id"`
	Kind       QuotaEventKind         `json:"kind"`
	RunID      string                 `json:"run_id,omitempty"`
	ResponseID string                 `json:"response_id,omitempty"`
	Action     string                 `json:"action,omitempty"`
	StrainCost int                    `json:"strain_cost,omitempty"`
	SparkCost  float64                `json:"spark_cost,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type RollingQuotaUsage struct {
	Window6HUsed int `json:"rolling_6h_used"`
	DayUsed      int `json:"rolling_day_used"`
	WeekUsed     int `json:"rolling_week_used"`
}

func (e QuotaEvent) CountsTowardRollingUsage() bool {
	switch e.Kind {
	case QuotaEventWorkCall, QuotaEventAcceptanceCall, QuotaEventFinalNotice:
		return e.StrainCost > 0
	default:
		return false
	}
}

func (s *Store) AppendQuotaEvent(event QuotaEvent) (string, error) {
	event.ResidentID = strings.TrimSpace(event.ResidentID)
	if event.ResidentID == "" {
		return "", fmt.Errorf("resident id is required")
	}
	if event.Kind == "" {
		return "", fmt.Errorf("quota event kind is required")
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	event.CreatedAt = event.CreatedAt.UTC()
	if event.ID == "" {
		event.ID = quotaEventID(event.Kind, event.CreatedAt)
	}

	path := s.quotaEventsPath(event.ResidentID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("mkdir quota event dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", fmt.Errorf("open quota events: %w", err)
	}
	defer f.Close()

	raw, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshal quota event: %w", err)
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return "", fmt.Errorf("write quota event: %w", err)
	}
	return path, nil
}

func (s *Store) LoadQuotaEvents(residentID string) ([]QuotaEvent, string, error) {
	residentID = strings.TrimSpace(residentID)
	if residentID == "" {
		return nil, "", fmt.Errorf("resident id is required")
	}
	path := s.quotaEventsPath(residentID)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, path, nil
		}
		return nil, path, fmt.Errorf("open quota events: %w", err)
	}
	defer f.Close()

	var events []QuotaEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event QuotaEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, path, fmt.Errorf("unmarshal quota event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, path, fmt.Errorf("scan quota events: %w", err)
	}
	return events, path, nil
}

func (s *Store) RollingQuotaUsage(residentID string, now time.Time) (RollingQuotaUsage, error) {
	events, _, err := s.LoadQuotaEvents(residentID)
	if err != nil {
		return RollingQuotaUsage{}, err
	}
	return RollingQuotaUsageFromEvents(events, now), nil
}

func RollingQuotaUsageFromEvents(events []QuotaEvent, now time.Time) RollingQuotaUsage {
	now = now.UTC()
	window6HSince := now.Add(-6 * time.Hour)
	daySince := now.Add(-24 * time.Hour)
	weekSince := now.Add(-7 * 24 * time.Hour)

	var out RollingQuotaUsage
	for _, event := range events {
		if !event.CountsTowardRollingUsage() {
			continue
		}
		createdAt := event.CreatedAt.UTC()
		if createdAt.After(now) {
			continue
		}
		if createdAt.After(window6HSince) {
			out.Window6HUsed += event.StrainCost
		}
		if createdAt.After(daySince) {
			out.DayUsed += event.StrainCost
		}
		if createdAt.After(weekSince) {
			out.WeekUsed += event.StrainCost
		}
	}
	return out
}

func (s *Store) quotaEventsPath(residentID string) string {
	return filepath.Join(s.rootDir, residentID, "quota-events.jsonl")
}

func quotaEventID(kind QuotaEventKind, at time.Time) string {
	stamp := at.UTC().Format("20060102T150405.000000000Z")
	return fmt.Sprintf("quota-%s-%s", kind, stamp)
}
