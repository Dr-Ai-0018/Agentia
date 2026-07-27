package newborn

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRoundJournalCrashHelper(t *testing.T) {
	if os.Getenv("ARENA_ROUND_JOURNAL_CRASH_HELPER") != "1" {
		return
	}
	outDir := os.Getenv("ARENA_ROUND_JOURNAL_OUT_DIR")
	started := time.Date(2026, 7, 27, 7, 30, 0, 0, time.UTC)
	writer := NewReportWriter()
	if err := writer.Begin(outDir, started, "jade"); err != nil {
		os.Exit(2)
	}
	if err := writer.AppendRound(outDir, started, "jade", RoundLog{Round: 1, Observation: "survives-sigkill", ResponseID: "resp-crash"}); err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(filepath.Join(outDir, "ready"), []byte("ready"), 0o644); err != nil {
		os.Exit(4)
	}
	time.Sleep(30 * time.Second)
}

func TestReportWriterRoundSurvivesProcessKillWithoutFinalize(t *testing.T) {
	outDir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRoundJournalCrashHelper$")
	cmd.Env = append(os.Environ(),
		"ARENA_ROUND_JOURNAL_CRASH_HELPER=1",
		"ARENA_ROUND_JOURNAL_OUT_DIR="+outDir,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start crash helper: %v", err)
	}
	readyPath := filepath.Join(outDir, "ready")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
			t.Fatal("crash helper did not finish durable append")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill crash helper: %v", err)
	}
	_, _ = cmd.Process.Wait()

	started := time.Date(2026, 7, 27, 7, 30, 0, 0, time.UTC)
	rounds, err := NewReportWriter().ReadRounds(outDir, started, "jade")
	if err != nil {
		t.Fatalf("read journal after process kill: %v", err)
	}
	if len(rounds) != 1 || rounds[0].Observation != "survives-sigkill" {
		t.Fatalf("unexpected rounds after process kill: %#v", rounds)
	}
	if _, err := os.Stat(filepath.Join(outDir, "jade-20260727T073000Z", "report.json")); !os.IsNotExist(err) {
		t.Fatalf("killed helper must not have a final report, stat err=%v", err)
	}
}

func TestReportWriterPersistsAppendOnlyRoundsAcrossWriterRestart(t *testing.T) {
	outDir := t.TempDir()
	started := time.Date(2026, 7, 27, 7, 0, 0, 0, time.UTC)
	writer := NewReportWriter()
	if err := writer.Begin(outDir, started, "jade"); err != nil {
		t.Fatalf("begin journal: %v", err)
	}
	want := []RoundLog{
		{Round: 1, RemainingSec: 120, Decision: AgentDecision{NextAction: "noop"}, Observation: "first", ResponseID: "resp-1"},
		{Round: 2, RemainingSec: 90, Decision: AgentDecision{NextAction: "guest_exec", Command: "whoami"}, Observation: "jade", ResponseID: "resp-2"},
	}
	for _, round := range want {
		if err := writer.AppendRound(outDir, started, "jade", round); err != nil {
			t.Fatalf("append round %d: %v", round.Round, err)
		}
	}

	// A fresh reader simulates inspecting the journal after the runner process died.
	got, err := NewReportWriter().ReadRounds(outDir, started, "jade")
	if err != nil {
		t.Fatalf("read journal after writer restart: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("durable rounds mismatch\n got: %#v\nwant: %#v", got, want)
	}
	if _, err := os.Stat(filepath.Join(outDir, "jade-20260727T070000Z", "report.json")); !os.IsNotExist(err) {
		t.Fatalf("interrupted run should preserve journal without requiring final report, stat err=%v", err)
	}
}

func TestReportWriterRejectsDuplicateOrOutOfOrderRound(t *testing.T) {
	outDir := t.TempDir()
	started := time.Date(2026, 7, 27, 7, 0, 0, 0, time.UTC)
	writer := NewReportWriter()
	if err := writer.Begin(outDir, started, "amber"); err != nil {
		t.Fatalf("begin journal: %v", err)
	}
	if err := writer.AppendRound(outDir, started, "amber", RoundLog{Round: 2}); err != nil {
		t.Fatalf("append first round: %v", err)
	}
	if err := writer.AppendRound(outDir, started, "amber", RoundLog{Round: 2}); err == nil || !strings.Contains(err.Error(), "strictly increasing") {
		t.Fatalf("expected duplicate round rejection, got %v", err)
	}
	if err := writer.AppendRound(outDir, started, "amber", RoundLog{Round: 1}); err == nil || !strings.Contains(err.Error(), "strictly increasing") {
		t.Fatalf("expected out-of-order round rejection, got %v", err)
	}
}

func TestReportWriterRejectsPartialJournalTail(t *testing.T) {
	outDir := t.TempDir()
	started := time.Date(2026, 7, 27, 7, 0, 0, 0, time.UTC)
	writer := NewReportWriter()
	if err := writer.Begin(outDir, started, "onyx"); err != nil {
		t.Fatalf("begin journal: %v", err)
	}
	if err := writer.AppendRound(outDir, started, "onyx", RoundLog{Round: 1}); err != nil {
		t.Fatalf("append round: %v", err)
	}
	path := filepath.Join(outDir, "onyx-20260727T070000Z", "rounds.jsonl")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	if _, err := file.WriteString(`{"round":2`); err != nil {
		t.Fatalf("write partial tail: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close journal: %v", err)
	}
	if _, err := NewReportWriter().ReadRounds(outDir, started, "onyx"); err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("expected partial tail error, got %v", err)
	}
}

func TestReportWriterValidateRoundsDetectsMemoryMismatch(t *testing.T) {
	outDir := t.TempDir()
	started := time.Date(2026, 7, 27, 7, 0, 0, 0, time.UTC)
	writer := NewReportWriter()
	if err := writer.Begin(outDir, started, "jade"); err != nil {
		t.Fatalf("begin journal: %v", err)
	}
	if err := writer.AppendRound(outDir, started, "jade", RoundLog{Round: 1, Observation: "durable"}); err != nil {
		t.Fatalf("append round: %v", err)
	}
	if _, err := writer.ValidateRounds(outDir, started, "jade", []RoundLog{{Round: 1, Observation: "memory"}}); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}
