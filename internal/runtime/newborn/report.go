package newborn

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

type ReportWriter struct {
	mu        sync.Mutex
	lastRound map[string]int
}

func NewReportWriter() *ReportWriter {
	return &ReportWriter{lastRound: map[string]int{}}
}

func (w *ReportWriter) Begin(outDir string, started time.Time, resident string) error {
	runDir := w.runDir(outDir, started, resident)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	journalPath := filepath.Join(runDir, "rounds.jsonl")
	file, err := os.OpenFile(journalPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create round journal: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync round journal: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close round journal: %w", err)
	}
	dir, err := os.Open(runDir)
	if err != nil {
		return fmt.Errorf("open run directory for sync: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync run directory: %w", err)
	}
	w.mu.Lock()
	w.lastRound[journalPath] = 0
	w.mu.Unlock()
	return nil
}

func (w *ReportWriter) AppendRound(outDir string, started time.Time, resident string, round RoundLog) error {
	if round.Round <= 0 {
		return fmt.Errorf("round number must be positive")
	}
	journalPath := filepath.Join(w.runDir(outDir, started, resident), "rounds.jsonl")
	w.mu.Lock()
	defer w.mu.Unlock()
	previous, ok := w.lastRound[journalPath]
	if !ok {
		return fmt.Errorf("round journal was not initialized")
	}
	if round.Round <= previous {
		return fmt.Errorf("round journal is not strictly increasing: previous=%d next=%d", previous, round.Round)
	}
	raw, err := json.Marshal(round)
	if err != nil {
		return fmt.Errorf("encode round journal entry: %w", err)
	}
	file, err := os.OpenFile(journalPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open round journal: %w", err)
	}
	line := append(raw, '\n')
	written, err := file.Write(line)
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("append round journal: %w", err)
	}
	if written != len(line) {
		_ = file.Close()
		return fmt.Errorf("append round journal: wrote %d of %d bytes", written, len(line))
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync round journal: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close round journal: %w", err)
	}
	w.lastRound[journalPath] = round.Round
	return nil
}

func (w *ReportWriter) ReadRounds(outDir string, started time.Time, resident string) ([]RoundLog, error) {
	path := filepath.Join(w.runDir(outDir, started, resident), "rounds.jsonl")
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open round journal: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	rounds := []RoundLog{}
	lineNumber := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineNumber++
			if line[len(line)-1] != '\n' {
				return nil, fmt.Errorf("round journal line %d is unterminated", lineNumber)
			}
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				return nil, fmt.Errorf("round journal line %d is empty", lineNumber)
			}
			var round RoundLog
			if err := json.Unmarshal(line, &round); err != nil {
				return nil, fmt.Errorf("decode round journal line %d: %w", lineNumber, err)
			}
			if round.Round <= 0 {
				return nil, fmt.Errorf("round journal line %d has invalid round %d", lineNumber, round.Round)
			}
			if len(rounds) > 0 && round.Round <= rounds[len(rounds)-1].Round {
				return nil, fmt.Errorf("round journal line %d is not strictly increasing: previous=%d current=%d", lineNumber, rounds[len(rounds)-1].Round, round.Round)
			}
			rounds = append(rounds, round)
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, fmt.Errorf("read round journal: %w", readErr)
		}
	}
	return rounds, nil
}

func (w *ReportWriter) ValidateRounds(outDir string, started time.Time, resident string, inMemory []RoundLog) ([]RoundLog, error) {
	durable, err := w.ReadRounds(outDir, started, resident)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(durable, inMemory) {
		return nil, fmt.Errorf("round journal mismatch: durable=%d in_memory=%d", len(durable), len(inMemory))
	}
	return durable, nil
}

func (w *ReportWriter) Write(outDir string, started time.Time, report FinalReport) error {
	runDir := w.runDir(outDir, started, report.Resident)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := filepath.Join(runDir, "report.json.tmp")
	finalPath := filepath.Join(runDir, "report.json")
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	written, err := file.Write(raw)
	if err != nil {
		_ = file.Close()
		return err
	}
	if written != len(raw) {
		_ = file.Close()
		return fmt.Errorf("write final report: wrote %d of %d bytes", written, len(raw))
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return err
	}
	dir, err := os.Open(runDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (w *ReportWriter) runDir(outDir string, started time.Time, resident string) string {
	return filepath.Join(strings.TrimSpace(outDir), strings.TrimSpace(resident)+"-"+started.Format("20060102T150405Z"))
}
