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

type SleepDepth string

const (
	SleepDepthAwake SleepDepth = "awake"
	SleepDepthRest  SleepDepth = "rest"
	SleepDepthSleep SleepDepth = "sleep"
	SleepDepthDeep  SleepDepth = "deep_sleep"
)

type SleepSession struct {
	ID             string     `json:"id"`
	ResidentID     string     `json:"resident_id"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        time.Time  `json:"ended_at,omitempty"`
	PlannedMinutes int        `json:"planned_minutes,omitempty"`
	ActualMinutes  int        `json:"actual_minutes,omitempty"`
	Depth          SleepDepth `json:"depth"`
	Reason         string     `json:"reason,omitempty"`
}

type SleepStateSnapshot struct {
	Depth                SleepDepth `json:"depth"`
	DebtHours            float64    `json:"debt_hours"`
	Active               bool       `json:"active,omitempty"`
	StartedAt            *time.Time `json:"started_at,omitempty"`
	PlannedWakeAt        *time.Time `json:"planned_wake_at,omitempty"`
	LastCompletedAt      *time.Time `json:"last_completed_at,omitempty"`
	LastActualMinutes    int        `json:"last_actual_minutes,omitempty"`
	LastDepth            SleepDepth `json:"last_depth,omitempty"`
	WeightedSleepLast72H float64    `json:"weighted_sleep_last_72h"`
}

type SleepDebtComputation struct {
	WeightedHours72H float64 `json:"weighted_hours_72h"`
	DebtHours        float64 `json:"debt_hours"`
}

const sleepDebtTargetHoursPerDay = 7.0

func (s *Store) RecordSleepStart(residentID string, plannedMinutes int, startedAt time.Time, reason string) (SleepSession, string, error) {
	residentID = strings.TrimSpace(residentID)
	if residentID == "" {
		return SleepSession{}, "", fmt.Errorf("resident id is required")
	}
	if plannedMinutes < 0 {
		plannedMinutes = 0
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	startedAt = startedAt.UTC()

	session := SleepSession{
		ID:             sleepEventID("sleep", startedAt),
		ResidentID:     residentID,
		StartedAt:      startedAt,
		PlannedMinutes: plannedMinutes,
		Depth:          SleepDepthRest,
		Reason:         strings.TrimSpace(reason),
	}
	path := s.activeSleepPath(residentID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return SleepSession{}, "", fmt.Errorf("mkdir sleep dir: %w", err)
	}
	raw, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return SleepSession{}, "", fmt.Errorf("marshal active sleep: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return SleepSession{}, "", fmt.Errorf("write active sleep: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return SleepSession{}, "", fmt.Errorf("rename active sleep: %w", err)
	}
	return session, path, nil
}

func (s *Store) RecordSleepEnd(residentID string, endedAt time.Time) (SleepSession, string, error) {
	residentID = strings.TrimSpace(residentID)
	if residentID == "" {
		return SleepSession{}, "", fmt.Errorf("resident id is required")
	}
	active, activePath, err := s.LoadActiveSleep(residentID)
	if err != nil {
		return SleepSession{}, "", err
	}
	if active.ResidentID == "" {
		return SleepSession{}, "", fmt.Errorf("no active sleep session for resident %s", residentID)
	}
	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	}
	endedAt = endedAt.UTC()
	if endedAt.Before(active.StartedAt) {
		endedAt = active.StartedAt
	}

	active.EndedAt = endedAt
	active.ActualMinutes = int(endedAt.Sub(active.StartedAt).Minutes())
	if active.ActualMinutes < 0 {
		active.ActualMinutes = 0
	}
	prior, err := s.RollingWeightedSleep(residentID, endedAt, 24*time.Hour)
	if err != nil {
		return SleepSession{}, "", err
	}
	active.Depth = ClassifySleepDepth(time.Duration(active.ActualMinutes)*time.Minute, prior.WeightedHours)

	path, err := s.AppendSleepSession(active)
	if err != nil {
		return SleepSession{}, "", err
	}
	if err := os.Remove(activePath); err != nil && !os.IsNotExist(err) {
		return SleepSession{}, "", fmt.Errorf("remove active sleep: %w", err)
	}
	return active, path, nil
}

func (s *Store) AppendSleepSession(session SleepSession) (string, error) {
	session.ResidentID = strings.TrimSpace(session.ResidentID)
	if session.ResidentID == "" {
		return "", fmt.Errorf("resident id is required")
	}
	if session.StartedAt.IsZero() {
		return "", fmt.Errorf("sleep started_at is required")
	}
	session.StartedAt = session.StartedAt.UTC()
	if !session.EndedAt.IsZero() {
		session.EndedAt = session.EndedAt.UTC()
	}
	if session.ID == "" {
		session.ID = sleepEventID("sleep", session.StartedAt)
	}
	if session.Depth == "" {
		session.Depth = SleepDepthRest
	}

	path := s.sleepEventsPath(session.ResidentID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("mkdir sleep event dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", fmt.Errorf("open sleep events: %w", err)
	}
	defer f.Close()
	raw, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("marshal sleep event: %w", err)
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return "", fmt.Errorf("write sleep event: %w", err)
	}
	return path, nil
}

func (s *Store) LoadSleepSessions(residentID string) ([]SleepSession, string, error) {
	residentID = strings.TrimSpace(residentID)
	if residentID == "" {
		return nil, "", fmt.Errorf("resident id is required")
	}
	path := s.sleepEventsPath(residentID)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, path, nil
		}
		return nil, path, fmt.Errorf("open sleep events: %w", err)
	}
	defer f.Close()

	var sessions []SleepSession
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var session SleepSession
		if err := json.Unmarshal([]byte(line), &session); err != nil {
			return nil, path, fmt.Errorf("unmarshal sleep event: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := scanner.Err(); err != nil {
		return nil, path, fmt.Errorf("scan sleep events: %w", err)
	}
	return sessions, path, nil
}

func (s *Store) LoadActiveSleep(residentID string) (SleepSession, string, error) {
	residentID = strings.TrimSpace(residentID)
	if residentID == "" {
		return SleepSession{}, "", fmt.Errorf("resident id is required")
	}
	path := s.activeSleepPath(residentID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SleepSession{}, path, nil
		}
		return SleepSession{}, path, fmt.Errorf("read active sleep: %w", err)
	}
	var session SleepSession
	if err := json.Unmarshal(raw, &session); err != nil {
		return SleepSession{}, path, fmt.Errorf("unmarshal active sleep: %w", err)
	}
	return session, path, nil
}

type RollingWeightedSleepResult struct {
	WeightedHours float64
	ActualMinutes int
	ObservedSince time.Time
}

func (s *Store) RollingWeightedSleep(residentID string, now time.Time, window time.Duration) (RollingWeightedSleepResult, error) {
	sessions, _, err := s.LoadSleepSessions(residentID)
	if err != nil {
		return RollingWeightedSleepResult{}, err
	}
	return RollingWeightedSleepFromSessions(sessions, now, window), nil
}

func RollingWeightedSleepFromSessions(sessions []SleepSession, now time.Time, window time.Duration) RollingWeightedSleepResult {
	now = now.UTC()
	since := now.Add(-window)
	var out RollingWeightedSleepResult
	for _, session := range sessions {
		if session.StartedAt.IsZero() || session.EndedAt.IsZero() || session.EndedAt.After(now) {
			continue
		}
		if !session.EndedAt.After(since) {
			continue
		}
		actualMinutes := session.ActualMinutes
		if actualMinutes <= 0 {
			actualMinutes = int(session.EndedAt.Sub(session.StartedAt).Minutes())
		}
		if actualMinutes <= 0 {
			continue
		}
		observedSince := session.StartedAt.UTC()
		if observedSince.Before(since) {
			observedSince = since
		}
		if out.ObservedSince.IsZero() || observedSince.Before(out.ObservedSince) {
			out.ObservedSince = observedSince
		}
		out.ActualMinutes += actualMinutes
		out.WeightedHours += (float64(actualMinutes) / 60.0) * sleepDepthWeight(session.Depth)
	}
	return out
}

func (s *Store) SleepDebt(residentID string, now time.Time) (SleepDebtComputation, error) {
	weighted, err := s.RollingWeightedSleep(residentID, now, 72*time.Hour)
	if err != nil {
		return SleepDebtComputation{}, err
	}
	observedHours := 0.0
	if !weighted.ObservedSince.IsZero() && weighted.ObservedSince.Before(now) {
		observedHours = now.Sub(weighted.ObservedSince).Hours()
	}
	target := sleepDebtTargetHours(observedHours)
	debt := target - weighted.WeightedHours
	if debt < 0 {
		debt = 0
	}
	return SleepDebtComputation{
		WeightedHours72H: roundFloat2(weighted.WeightedHours),
		DebtHours:        roundFloat2(debt),
	}, nil
}

func sleepDebtTargetHours(observedHours float64) float64 {
	if observedHours <= 0 {
		return 0
	}
	target := (observedHours / 24.0) * sleepDebtTargetHoursPerDay
	if target > sleepDebtTargetHoursPerDay*3 {
		return sleepDebtTargetHoursPerDay * 3
	}
	return target
}

func (s *Store) CurrentSleepState(residentID string, now time.Time) SleepStateSnapshot {
	active, _, _ := s.LoadActiveSleep(residentID)
	debt, _ := s.SleepDebt(residentID, now)
	state := SleepStateSnapshot{
		Depth:                SleepDepthAwake,
		DebtHours:            debt.DebtHours,
		WeightedSleepLast72H: debt.WeightedHours72H,
	}
	if active.ResidentID != "" {
		started := active.StartedAt.UTC()
		wake := started.Add(time.Duration(active.PlannedMinutes) * time.Minute)
		prior, _ := s.RollingWeightedSleep(residentID, now, 24*time.Hour)
		state.Depth = ClassifySleepDepth(now.Sub(started), prior.WeightedHours)
		state.Active = true
		state.StartedAt = &started
		state.PlannedWakeAt = &wake
		return state
	}
	sessions, _, err := s.LoadSleepSessions(residentID)
	if err != nil || len(sessions) == 0 {
		return state
	}
	last := sessions[len(sessions)-1]
	if !last.EndedAt.IsZero() {
		ended := last.EndedAt.UTC()
		state.LastCompletedAt = &ended
		state.LastActualMinutes = last.ActualMinutes
		state.LastDepth = last.Depth
	}
	return state
}

func ClassifySleepDepth(duration time.Duration, priorWeightedSleep24H float64) SleepDepth {
	minutes := duration.Minutes()
	switch {
	case minutes < 5:
		return SleepDepthRest
	case minutes < 30:
		return SleepDepthRest
	case minutes < 240:
		return SleepDepthSleep
	case priorWeightedSleep24H+minutes/60.0 >= 6:
		return SleepDepthDeep
	default:
		return SleepDepthSleep
	}
}

func RecoveryModeForSleepDepth(depth SleepDepth) string {
	switch depth {
	case SleepDepthRest:
		return "rest"
	case SleepDepthSleep:
		return "sleep"
	case SleepDepthDeep:
		return "deep_sleep"
	default:
		return "idle"
	}
}

func sleepDepthWeight(depth SleepDepth) float64 {
	switch depth {
	case SleepDepthRest:
		return 0.35
	case SleepDepthSleep:
		return 1.0
	case SleepDepthDeep:
		return 1.25
	default:
		return 0
	}
}

func sleepEventID(kind string, at time.Time) string {
	stamp := at.UTC().Format("20060102T150405.000000000Z")
	return fmt.Sprintf("%s-%s", kind, stamp)
}

func (s *Store) sleepEventsPath(residentID string) string {
	return filepath.Join(s.rootDir, residentID, "sleep-events.jsonl")
}

func (s *Store) activeSleepPath(residentID string) string {
	return filepath.Join(s.rootDir, residentID, "sleep-active.json")
}

func roundFloat2(v float64) float64 {
	if v < 0 {
		v = 0
	}
	return float64(int(v*100+0.5)) / 100
}
