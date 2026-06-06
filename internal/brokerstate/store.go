package brokerstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"ai-arena/internal/runtimecore"
)

type Store struct {
	rootDir string
}

func New(rootDir string) *Store {
	return &Store{rootDir: rootDir}
}

func (s *Store) SaveResidentSnapshot(residentID string, snapshot runtimecore.Snapshot) (string, error) {
	dir := filepath.Join(s.rootDir, residentID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir resident dir: %w", err)
	}

	path := filepath.Join(dir, "runtime-state.json")
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
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
