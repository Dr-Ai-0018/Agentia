package brokerstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-arena/internal/runtimecore"
)

var ErrSnapshotVersionConflict = errors.New("snapshot version conflict")

type Store struct {
	rootDir string
}

func New(rootDir string) *Store {
	return &Store{rootDir: rootDir}
}

func (s *Store) SaveResidentSnapshot(residentID string, snapshot runtimecore.Snapshot) (string, error) {
	current, _, err := s.LoadResidentSnapshot(residentID)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	expected := uint64(0)
	if err == nil {
		expected = current.Revision
	}
	return s.SaveResidentSnapshotCAS(residentID, snapshot, expected)
}

func (s *Store) SaveResidentSnapshotCAS(residentID string, snapshot runtimecore.Snapshot, expectedRevision uint64) (string, error) {
	dir := filepath.Join(s.rootDir, residentID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir resident dir: %w", err)
	}

	path := filepath.Join(dir, "runtime-state.json")
	currentRevision := uint64(0)
	rawExisting, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read current snapshot: %w", err)
		}
		if expectedRevision != 0 {
			return "", fmt.Errorf("%w: resident=%s expected=%d actual=missing", ErrSnapshotVersionConflict, residentID, expectedRevision)
		}
	} else {
		var current runtimecore.Snapshot
		if err := json.Unmarshal(rawExisting, &current); err != nil {
			return "", fmt.Errorf("unmarshal current snapshot: %w", err)
		}
		currentRevision = current.Revision
		if currentRevision != expectedRevision {
			return "", fmt.Errorf("%w: resident=%s expected=%d actual=%d", ErrSnapshotVersionConflict, residentID, expectedRevision, currentRevision)
		}
	}

	snapshot.Revision = currentRevision + 1
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("rename snapshot: %w", err)
	}
	return path, nil
}

func (s *Store) LoadResidentSnapshot(residentID string) (runtimecore.Snapshot, string, error) {
	path := filepath.Join(s.rootDir, residentID, "runtime-state.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return runtimecore.Snapshot{}, path, err
	}

	var snapshot runtimecore.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return runtimecore.Snapshot{}, path, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return snapshot, path, nil
}

func (s *Store) DeleteResidentSnapshot(residentID string) error {
	path := filepath.Join(s.rootDir, residentID, "runtime-state.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) QuarantineResidentSnapshot(residentID string, reason string, now time.Time) (string, error) {
	path := filepath.Join(s.rootDir, residentID, "runtime-state.json")
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	stamp := now.UTC().Format("20060102T150405.000000000Z")
	suffix := "quarantine"
	if trimmed := filepath.Clean(reason); trimmed != "." && trimmed != "/" {
		replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", "\t", "-", "\n", "-")
		candidate := replacer.Replace(trimmed)
		candidate = strings.Trim(candidate, "-")
		if candidate != "" {
			suffix = candidate
		}
	}
	dst := filepath.Join(s.rootDir, residentID, fmt.Sprintf("runtime-state.%s.%s.json", stamp, suffix))
	if err := os.Rename(path, dst); err != nil {
		return "", err
	}
	return dst, nil
}
