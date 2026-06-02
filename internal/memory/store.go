package memory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RecordStatus string

const (
	StatusActive   RecordStatus = "active"
	StatusDecaying RecordStatus = "decaying"
	StatusReview   RecordStatus = "review"
	StatusDeleted  RecordStatus = "deleted"
)

type Record struct {
	ID          string       `json:"id"`
	Layer       Layer        `json:"layer"`
	Domain      Domain       `json:"domain"`
	Status      RecordStatus `json:"status"`
	ReasonCodes []string     `json:"reason_codes"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	ReviewAfter time.Time    `json:"review_after"`
	ExpiresAt   time.Time    `json:"expires_at"`
	Pinned      bool         `json:"pinned"`
}

type StoreRecord struct {
	Record
	Resident       string `json:"resident"`
	Summary        string `json:"summary"`
	DecisionAction Action `json:"decision_action"`
	SourceRunID    string `json:"source_run_id,omitempty"`
}

type SnapshotEntry struct {
	ID             string `json:"id"`
	Layer          Layer  `json:"layer"`
	DecisionAction Action `json:"decision_action"`
	Summary        string `json:"summary"`
}

type Store interface {
	List(resident string) ([]StoreRecord, error)
	Upsert(record StoreRecord) error
	Get(resident, id string) (StoreRecord, bool, error)
}

type MemoryStore struct {
	records map[string][]StoreRecord
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		records: make(map[string][]StoreRecord),
	}
}

func (s *MemoryStore) List(resident string) ([]StoreRecord, error) {
	records := append([]StoreRecord(nil), s.records[resident]...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].UpdatedAt.Equal(records[j].UpdatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
	return records, nil
}

func (s *MemoryStore) Get(resident, id string) (StoreRecord, bool, error) {
	for _, record := range s.records[resident] {
		if record.ID == id {
			return record, true, nil
		}
	}
	return StoreRecord{}, false, nil
}

func (s *MemoryStore) Upsert(record StoreRecord) error {
	if strings.TrimSpace(record.Resident) == "" {
		return errors.New("resident is required")
	}
	if strings.TrimSpace(record.ID) == "" {
		return errors.New("record id is required")
	}

	list := s.records[record.Resident]
	for i := range list {
		if list[i].ID == record.ID {
			list[i] = record
			s.records[record.Resident] = list
			return nil
		}
	}
	s.records[record.Resident] = append(list, record)
	return nil
}

func BuildSnapshot(records []StoreRecord, limit int) []SnapshotEntry {
	if limit <= 0 || limit > len(records) {
		limit = len(records)
	}
	entries := make([]SnapshotEntry, 0, limit)
	for _, record := range records[:limit] {
		if record.Status == StatusDeleted {
			continue
		}
		entries = append(entries, SnapshotEntry{
			ID:             record.ID,
			Layer:          record.Layer,
			DecisionAction: record.DecisionAction,
			Summary:        record.Summary,
		})
	}
	return entries
}

type FileStore struct {
	root string
}

func NewFileStore(root string) *FileStore {
	return &FileStore{root: root}
}

func (s *FileStore) List(resident string) ([]StoreRecord, error) {
	path := s.residentPath(resident)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var records []StoreRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].UpdatedAt.Equal(records[j].UpdatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
	return records, nil
}

func (s *FileStore) Get(resident, id string) (StoreRecord, bool, error) {
	records, err := s.List(resident)
	if err != nil {
		return StoreRecord{}, false, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, true, nil
		}
	}
	return StoreRecord{}, false, nil
}

func (s *FileStore) Upsert(record StoreRecord) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return err
	}
	records, err := s.List(record.Resident)
	if err != nil {
		return err
	}
	found := false
	for i := range records {
		if records[i].ID == record.ID {
			records[i] = record
			found = true
			break
		}
	}
	if !found {
		records = append(records, record)
	}
	raw, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.residentPath(record.Resident), raw, 0o644)
}

func (s *FileStore) residentPath(resident string) string {
	return filepath.Join(s.root, resident+".json")
}

func ApplyDecision(now time.Time, record Record, decision Decision) Record {
	record.UpdatedAt = now
	record.Layer = decision.TargetLayer

	switch decision.Action {
	case ActionCreate, ActionPromote, ActionRetain, ActionUpdate:
		record.Status = StatusActive
	case ActionDecay:
		record.Status = StatusDecaying
	case ActionReview:
		record.Status = StatusReview
	case ActionDelete:
		record.Status = StatusDeleted
	}
	record.ReasonCodes = append([]string(nil), decision.ReasonCodes...)

	if decision.TTL > 0 {
		record.ExpiresAt = now.Add(decision.TTL)
	}
	if decision.ReviewAfter > 0 {
		record.ReviewAfter = now.Add(decision.ReviewAfter)
	}

	return record
}
