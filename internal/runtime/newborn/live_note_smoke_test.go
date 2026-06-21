//go:build live

package newborn

import (
	"bufio"
	"bytes"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"ai-arena/internal/openai"
)

func TestLiveNoteAPISmoke(t *testing.T) {
	instance := strings.TrimSpace(os.Getenv("AI_ARENA_LIVE_INSTANCE"))
	resident := strings.TrimSpace(os.Getenv("AI_ARENA_LIVE_RESIDENT"))
	if instance == "" {
		instance = "jade"
	}
	if resident == "" {
		resident = instance
	}
	profile := ResidentProfile{Name: resident, Instance: instance}
	executor := &IncusActionExecutor{}
	noteFile := "codex-live-note-api-smoke.md"
	stamp := time.Now().UTC().Format(time.RFC3339)

	denied := executor.Execute(profile, AgentDecision{
		NextAction: "guest_exec",
		Command:    "cat /root/arena-notes/" + noteFile,
		Reason:     "live smoke verifies continuity surface boundary",
	})
	if !denied.Error || denied.ErrorKind != "continuity_surface_requires_note_tool" {
		t.Fatalf("expected semantic note-tool denial, got %#v", denied)
	}

	appendResult := executor.Execute(profile, AgentDecision{
		NextAction: "note_append",
		NoteFile:   noteFile,
		NoteText:   "live smoke append via note_append at " + stamp,
		Reason:     "verify append path",
	})
	requireLiveActionOK(t, "note_append", appendResult)

	readResult := executor.Execute(profile, AgentDecision{
		NextAction: "note_read",
		NoteFile:   noteFile,
		Reason:     "verify read path",
	})
	requireLiveActionOK(t, "note_read", readResult)
	if !strings.Contains(readResult.Observation, "live smoke append via note_append") {
		t.Fatalf("note_read did not include appended text: %s", readResult.Observation)
	}

	replaceResult := executor.Execute(profile, AgentDecision{
		NextAction: "note_replace_with_backup",
		NoteFile:   noteFile,
		NoteText:   "live smoke replacement via note_replace_with_backup at " + stamp,
		Reason:     "verify backup replace path",
	})
	requireLiveActionOK(t, "note_replace_with_backup", replaceResult)
	backup := backupNameFromObservation(replaceResult.Observation)
	if backup == "" {
		t.Fatalf("replace observation did not report backup file: %s", replaceResult.Observation)
	}

	compactResult := executor.Execute(profile, AgentDecision{
		NextAction: "note_summarize_or_compact",
		NoteFile:   noteFile,
		NoteText:   "live smoke compacted continuity summary at " + stamp,
		Reason:     "verify compact path",
	})
	requireLiveActionOK(t, "note_summarize_or_compact", compactResult)

	restoreResult := executor.Execute(profile, AgentDecision{
		NextAction: "note_restore_backup",
		NoteFile:   noteFile,
		BackupFile: backup,
		Reason:     "verify restore path",
	})
	requireLiveActionOK(t, "note_restore_backup", restoreResult)

	restoredRead := executor.Execute(profile, AgentDecision{
		NextAction: "note_read",
		NoteFile:   noteFile,
		Reason:     "verify restored content",
	})
	requireLiveActionOK(t, "note_read restored", restoredRead)
	if !strings.Contains(restoredRead.Observation, "live smoke append via note_append") {
		t.Fatalf("restored note did not include original appended text: %s", restoredRead.Observation)
	}
}

func TestLiveModelChoosesNoteReadForContinuitySurface(t *testing.T) {
	loadLiveDotEnv(t)
	apiKey := strings.TrimSpace(os.Getenv("JADE_OPENAI_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is required for live model decision smoke")
	}
	baseURL := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	profile := ResidentProfile{Name: "jade", Model: "gpt-5.4", Instance: "jade"}
	input := []openai.Message{
		{
			Role: "user",
			Content: strings.Join([]string{
				"[live_smoke_context]",
				"Task: read the continuity file boot-notes.md under /root/arena-notes.",
				"Use the dedicated note API for continuity surfaces.",
				"Do not inspect any other surface first.",
			}, "\n"),
		},
	}
	result, err := openai.PostStream(&http.Client{Timeout: 2 * time.Minute}, baseURL, apiKey, buildDecisionToolPayload(profile, input, "live-note-tool-choice-smoke"), false)
	if err != nil {
		t.Fatalf("live model decision request failed: %v", err)
	}
	decision, err := parseDecisionResult(result)
	if err != nil {
		t.Fatalf("parse live model decision: %v", err)
	}
	if decision.NextAction != "note_read" {
		t.Fatalf("expected note_read for continuity read task, got action=%s command=%q note_file=%q calls=%#v", decision.NextAction, decision.Command, decision.NoteFile, result.FunctionCalls)
	}
	if decision.Command != "" {
		t.Fatalf("note_read decision must not carry shell command, got %q", decision.Command)
	}
}

func requireLiveActionOK(t *testing.T, label string, result ActionResult) {
	t.Helper()
	if result.Error {
		t.Fatalf("%s failed kind=%s observation=%s raw=%s", label, result.ErrorKind, result.Observation, result.RawOutput)
	}
}

func backupNameFromObservation(observation string) string {
	for _, line := range strings.Split(observation, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "backup=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "backup="))
		}
	}
	return ""
}

func loadLiveDotEnv(t *testing.T) {
	t.Helper()
	raw, err := readFirstExisting(".env", "../../../.env")
	if err != nil || len(raw) == 0 {
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		os.Setenv(key, value)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read .env: %v", err)
	}
}

func readFirstExisting(paths ...string) ([]byte, error) {
	var lastErr error
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err == nil {
			return raw, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
