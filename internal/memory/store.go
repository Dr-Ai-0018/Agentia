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
type HistoryGroupState string

const (
	StatusActive   RecordStatus = "active"
	StatusDecaying RecordStatus = "decaying"
	StatusReview   RecordStatus = "review"
	StatusDeleted  RecordStatus = "deleted"

	HistoryGroupOpen   HistoryGroupState = "open"
	HistoryGroupClosed HistoryGroupState = "closed"
)

type Record struct {
	ID              string       `json:"id"`
	Layer           Layer        `json:"layer"`
	Domain          Domain       `json:"domain"`
	Status          RecordStatus `json:"status"`
	ReasonCodes     []string     `json:"reason_codes"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
	LastAccessedAt  time.Time    `json:"last_accessed_at,omitempty"`
	LastConfirmedAt time.Time    `json:"last_confirmed_at,omitempty"`
	ReviewAt        time.Time    `json:"review_at,omitempty"`
	ReviewAfter     time.Time    `json:"review_after,omitempty"`
	ExpiresAt       time.Time    `json:"expires_at,omitempty"`
	HardExpiresAt   time.Time    `json:"hard_expires_at,omitempty"`
	Pinned          bool         `json:"pinned"`
}

type HistoryGroup struct {
	GroupUUID       string            `json:"group_uuid"`
	Resident        string            `json:"resident"`
	CreatedAt       time.Time         `json:"created_at"`
	ClosedAt        time.Time         `json:"closed_at"`
	LastEventAt     time.Time         `json:"last_event_at,omitempty"`
	SourceKind      string            `json:"source_kind"`
	State           HistoryGroupState `json:"state"`
	CloseReason     string            `json:"close_reason,omitempty"`
	EventCount      int               `json:"event_count"`
	Tags            []string          `json:"tags"`
	SummaryHint     string            `json:"summary_hint"`
	RawEventRefs    []string          `json:"raw_event_refs"`
	ExtractedLayers []string          `json:"extracted_layers,omitempty"`
}

type AbstractMemory struct {
	Record
	Resident        string         `json:"resident"`
	Summary         string         `json:"summary"`
	ResidentText    string         `json:"resident_text,omitempty"`
	Semantic        SemanticMemory `json:"semantic,omitempty"`
	DecisionAction  Action         `json:"decision_action"`
	SourceRunID     string         `json:"source_run_id,omitempty"`
	SourceGroupIDs  []string       `json:"source_group_ids"`
	ParentMemoryIDs []string       `json:"parent_memory_ids"`
	Boundary        string         `json:"boundary,omitempty"`
	Confidence      float64        `json:"confidence,omitempty"`
}

type SemanticMemory struct {
	MemoryKind      string `json:"memory_kind,omitempty"`
	Salience        int    `json:"salience,omitempty"`
	EmotionTone     string `json:"emotion_tone,omitempty"`
	TimeScope       string `json:"time_scope,omitempty"`
	RetentionIntent string `json:"retention_intent,omitempty"`
	DropCondition   string `json:"drop_condition,omitempty"`
}

type SnapshotEntry struct {
	ID              string `json:"id"`
	Layer           Layer  `json:"layer"`
	DecisionAction  Action `json:"decision_action"`
	Summary         string `json:"summary"`
	ResidentText    string `json:"resident_text,omitempty"`
	MemoryKind      string `json:"memory_kind,omitempty"`
	Salience        int    `json:"salience,omitempty"`
	EmotionTone     string `json:"emotion_tone,omitempty"`
	TimeScope       string `json:"time_scope,omitempty"`
	RetentionIntent string `json:"retention_intent,omitempty"`
	DropCondition   string `json:"drop_condition,omitempty"`
}

type ResidentMemoryBundle struct {
	HistoryGroups    []HistoryGroup   `json:"history_groups"`
	AbstractMemories []AbstractMemory `json:"abstract_memories"`
}

type Store interface {
	ListAbstractMemories(resident string) ([]AbstractMemory, error)
	UpsertAbstractMemory(record AbstractMemory) error
	GetAbstractMemory(resident, id string) (AbstractMemory, bool, error)
	ListHistoryGroups(resident string) ([]HistoryGroup, error)
	UpsertHistoryGroup(group HistoryGroup) error
	CompactResident(resident string) error
}

type MemoryStore struct {
	historyGroups    map[string][]HistoryGroup
	abstractMemories map[string][]AbstractMemory
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		historyGroups:    make(map[string][]HistoryGroup),
		abstractMemories: make(map[string][]AbstractMemory),
	}
}

func (s *MemoryStore) ListAbstractMemories(resident string) ([]AbstractMemory, error) {
	records := append([]AbstractMemory(nil), s.abstractMemories[resident]...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].UpdatedAt.Equal(records[j].UpdatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
	return records, nil
}

func (s *MemoryStore) GetAbstractMemory(resident, id string) (AbstractMemory, bool, error) {
	for _, record := range s.abstractMemories[resident] {
		if record.ID == id {
			return record, true, nil
		}
	}
	return AbstractMemory{}, false, nil
}

func (s *MemoryStore) UpsertAbstractMemory(record AbstractMemory) error {
	if strings.TrimSpace(record.Resident) == "" {
		return errors.New("resident is required")
	}
	if strings.TrimSpace(record.ID) == "" {
		return errors.New("abstract memory id is required")
	}
	list := s.abstractMemories[record.Resident]
	for i := range list {
		if list[i].ID == record.ID {
			list[i] = record
			s.abstractMemories[record.Resident] = list
			return nil
		}
	}
	s.abstractMemories[record.Resident] = append(list, record)
	return nil
}

func (s *MemoryStore) ListHistoryGroups(resident string) ([]HistoryGroup, error) {
	groups := append([]HistoryGroup(nil), s.historyGroups[resident]...)
	for i := range groups {
		groups[i] = normalizeHistoryGroup(groups[i])
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].CreatedAt.After(groups[j].CreatedAt)
	})
	return groups, nil
}

func (s *MemoryStore) UpsertHistoryGroup(group HistoryGroup) error {
	group = normalizeHistoryGroup(group)
	if strings.TrimSpace(group.Resident) == "" {
		return errors.New("resident is required")
	}
	if strings.TrimSpace(group.GroupUUID) == "" {
		return errors.New("group uuid is required")
	}
	list := s.historyGroups[group.Resident]
	for i := range list {
		if list[i].GroupUUID == group.GroupUUID {
			list[i] = group
			s.historyGroups[group.Resident] = list
			return nil
		}
	}
	s.historyGroups[group.Resident] = append(list, group)
	return nil
}

func (s *MemoryStore) CompactResident(resident string) error {
	groups, err := s.ListHistoryGroups(resident)
	if err != nil {
		return err
	}
	records, err := s.ListAbstractMemories(resident)
	if err != nil {
		return err
	}
	compactedGroups, groupIDMap := compactHistoryGroups(groups)
	compactedRecords := remapAbstractMemoryGroups(records, groupIDMap)
	s.historyGroups[resident] = compactedGroups
	s.abstractMemories[resident] = compactedRecords
	return nil
}

func BuildSnapshot(records []AbstractMemory, limit int) []SnapshotEntry {
	if limit <= 0 || limit > len(records) {
		limit = len(records)
	}
	entries := make([]SnapshotEntry, 0, limit)
	for _, record := range records[:limit] {
		if record.Status == StatusDeleted {
			continue
		}
		entries = append(entries, SnapshotEntry{
			ID:              record.ID,
			Layer:           record.Layer,
			DecisionAction:  record.DecisionAction,
			Summary:         record.EffectiveSummary(),
			ResidentText:    record.ResidentText,
			MemoryKind:      record.Semantic.MemoryKind,
			Salience:        record.Semantic.Salience,
			EmotionTone:     record.Semantic.EmotionTone,
			TimeScope:       record.Semantic.TimeScope,
			RetentionIntent: record.Semantic.RetentionIntent,
			DropCondition:   record.Semantic.DropCondition,
		})
	}
	return entries
}

func (m AbstractMemory) EffectiveSummary() string {
	if strings.TrimSpace(m.Summary) != "" {
		return strings.TrimSpace(m.Summary)
	}
	if strings.TrimSpace(m.ResidentText) != "" {
		text := strings.TrimSpace(m.ResidentText)
		if idx := strings.IndexAny(text, ".!?\n"); idx > 0 {
			return strings.TrimSpace(text[:idx])
		}
		return text
	}
	return ""
}

type FileStore struct {
	root string
}

func NewFileStore(root string) *FileStore {
	return &FileStore{root: root}
}

func (s *FileStore) ListAbstractMemories(resident string) ([]AbstractMemory, error) {
	bundle, err := s.loadBundle(resident)
	if err != nil {
		return nil, err
	}
	records := append([]AbstractMemory(nil), bundle.AbstractMemories...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].UpdatedAt.Equal(records[j].UpdatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
	return records, nil
}

func (s *FileStore) GetAbstractMemory(resident, id string) (AbstractMemory, bool, error) {
	records, err := s.ListAbstractMemories(resident)
	if err != nil {
		return AbstractMemory{}, false, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, true, nil
		}
	}
	return AbstractMemory{}, false, nil
}

func (s *FileStore) UpsertAbstractMemory(record AbstractMemory) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return err
	}
	bundle, err := s.loadBundle(record.Resident)
	if err != nil {
		return err
	}
	found := false
	for i := range bundle.AbstractMemories {
		if bundle.AbstractMemories[i].ID == record.ID {
			bundle.AbstractMemories[i] = record
			found = true
			break
		}
	}
	if !found {
		bundle.AbstractMemories = append(bundle.AbstractMemories, record)
	}
	return s.writeBundle(record.Resident, bundle)
}

func (s *FileStore) ListHistoryGroups(resident string) ([]HistoryGroup, error) {
	bundle, err := s.loadBundle(resident)
	if err != nil {
		return nil, err
	}
	groups := append([]HistoryGroup(nil), bundle.HistoryGroups...)
	for i := range groups {
		groups[i] = normalizeHistoryGroup(groups[i])
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].CreatedAt.After(groups[j].CreatedAt)
	})
	return groups, nil
}

func (s *FileStore) UpsertHistoryGroup(group HistoryGroup) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return err
	}
	group = normalizeHistoryGroup(group)
	bundle, err := s.loadBundle(group.Resident)
	if err != nil {
		return err
	}
	found := false
	for i := range bundle.HistoryGroups {
		if bundle.HistoryGroups[i].GroupUUID == group.GroupUUID {
			bundle.HistoryGroups[i] = group
			found = true
			break
		}
	}
	if !found {
		bundle.HistoryGroups = append(bundle.HistoryGroups, group)
	}
	return s.writeBundle(group.Resident, bundle)
}

func (s *FileStore) CompactResident(resident string) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return err
	}
	bundle, err := s.loadBundle(resident)
	if err != nil {
		return err
	}
	groups := append([]HistoryGroup(nil), bundle.HistoryGroups...)
	for i := range groups {
		groups[i] = normalizeHistoryGroup(groups[i])
	}
	compactedGroups, groupIDMap := compactHistoryGroups(groups)
	bundle.HistoryGroups = compactedGroups
	bundle.AbstractMemories = remapAbstractMemoryGroups(bundle.AbstractMemories, groupIDMap)
	return s.writeBundle(resident, bundle)
}

func (s *FileStore) loadBundle(resident string) (ResidentMemoryBundle, error) {
	path := s.residentPath(resident)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ResidentMemoryBundle{}, nil
		}
		return ResidentMemoryBundle{}, err
	}
	var bundle ResidentMemoryBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		var legacy []AbstractMemory
		if legacyErr := json.Unmarshal(raw, &legacy); legacyErr != nil {
			return ResidentMemoryBundle{}, err
		}
		bundle.AbstractMemories = legacy
	}
	return bundle, nil
}

func (s *FileStore) writeBundle(resident string, bundle ResidentMemoryBundle) error {
	raw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.residentPath(resident), raw, 0o644)
}

func (s *FileStore) residentPath(resident string) string {
	return filepath.Join(s.root, resident+".json")
}

func ApplyDecision(now time.Time, record Record, decision Decision) Record {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	record.Layer = decision.TargetLayer

	switch decision.Action {
	case ActionCreate, ActionPromote, ActionRetain, ActionUpdate:
		record.Status = StatusActive
		record.LastConfirmedAt = now
	case ActionDecay:
		record.Status = StatusDecaying
	case ActionReview:
		record.Status = StatusReview
	case ActionDelete:
		record.Status = StatusDeleted
	}
	record.ReasonCodes = append([]string(nil), decision.ReasonCodes...)

	if record.LastAccessedAt.IsZero() {
		record.LastAccessedAt = now
	}
	if decision.TTL > 0 {
		record.ExpiresAt = now.Add(decision.TTL)
	}
	if decision.ReviewAfter > 0 {
		record.ReviewAfter = now
		record.ReviewAt = now.Add(decision.ReviewAfter)
	}
	if record.HardExpiresAt.IsZero() || (!record.ExpiresAt.IsZero() && record.HardExpiresAt.Before(record.ExpiresAt)) {
		record.HardExpiresAt = deriveHardExpiry(now, record.Layer, record.ExpiresAt)
	}

	return record
}

func deriveHardExpiry(now time.Time, layer Layer, expiresAt time.Time) time.Time {
	base := expiresAt
	if base.IsZero() {
		base = now
	}
	switch layer {
	case LayerInstant:
		return base.Add(4 * time.Hour)
	case LayerShort:
		return base.Add(24 * time.Hour)
	case LayerLong:
		return base.Add(30 * 24 * time.Hour)
	case LayerPermanent:
		return base.Add(365 * 24 * time.Hour)
	default:
		return base.Add(24 * time.Hour)
	}
}

func (s *FileStore) Root() string {
	return s.root
}

func normalizeHistoryGroup(group HistoryGroup) HistoryGroup {
	if group.LastEventAt.IsZero() {
		if !group.ClosedAt.IsZero() {
			group.LastEventAt = group.ClosedAt
		} else {
			group.LastEventAt = group.CreatedAt
		}
	}
	if strings.TrimSpace(string(group.State)) == "" {
		if group.EventCount > 0 {
			group.State = HistoryGroupClosed
		} else {
			group.State = HistoryGroupOpen
		}
	}
	if group.State == HistoryGroupClosed && strings.TrimSpace(group.CloseReason) == "" {
		group.CloseReason = "legacy_closed_group"
	}
	if group.EventCount == 0 {
		group.EventCount = len(group.RawEventRefs)
	}
	return group
}

func compactHistoryGroups(groups []HistoryGroup) ([]HistoryGroup, map[string]string) {
	type candidate struct {
		group HistoryGroup
		index int
	}
	bySignature := make(map[string]candidate)
	idMap := make(map[string]string)
	for _, group := range groups {
		group = normalizeHistoryGroup(group)
		signature := historyGroupSignature(group)
		existing, ok := bySignature[signature]
		if !ok {
			bySignature[signature] = candidate{group: group}
			idMap[group.GroupUUID] = group.GroupUUID
			continue
		}
		merged := mergeHistoryGroup(existing.group, group)
		bySignature[signature] = candidate{group: merged}
		idMap[group.GroupUUID] = merged.GroupUUID
		idMap[existing.group.GroupUUID] = merged.GroupUUID
	}
	compacted := make([]HistoryGroup, 0, len(bySignature))
	for _, item := range bySignature {
		compacted = append(compacted, normalizeHistoryGroup(item.group))
	}
	sort.Slice(compacted, func(i, j int) bool {
		return compacted[i].CreatedAt.After(compacted[j].CreatedAt)
	})
	return compacted, idMap
}

func remapAbstractMemoryGroups(records []AbstractMemory, groupIDMap map[string]string) []AbstractMemory {
	out := make([]AbstractMemory, 0, len(records))
	for _, record := range records {
		record.SourceGroupIDs = remapGroupIDs(record.SourceGroupIDs, groupIDMap)
		out = append(out, record)
	}
	return out
}

func remapGroupIDs(groupIDs []string, groupIDMap map[string]string) []string {
	var mapped []string
	seen := map[string]struct{}{}
	for _, groupID := range groupIDs {
		target := groupID
		if replacement, ok := groupIDMap[groupID]; ok && strings.TrimSpace(replacement) != "" {
			target = replacement
		}
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		mapped = append(mapped, target)
	}
	return mapped
}

func historyGroupSignature(group HistoryGroup) string {
	return strings.Join(group.RawEventRefs, "\n")
}

func mergeHistoryGroup(left, right HistoryGroup) HistoryGroup {
	left = normalizeHistoryGroup(left)
	right = normalizeHistoryGroup(right)
	keep := left
	drop := right
	if preferHistoryGroup(right, left) {
		keep = right
		drop = left
	}
	keep.CreatedAt = minTime(left.CreatedAt, right.CreatedAt)
	keep.ClosedAt = maxTime(left.ClosedAt, right.ClosedAt)
	keep.LastEventAt = maxTime(left.LastEventAt, right.LastEventAt)
	keep.EventCount = maxInt(left.EventCount, right.EventCount)
	keep.Tags = mergeStringSlices(left.Tags, right.Tags)
	if strings.TrimSpace(keep.SummaryHint) == "" {
		keep.SummaryHint = drop.SummaryHint
	}
	keep.RawEventRefs = mergeStringSlices(left.RawEventRefs, right.RawEventRefs)
	keep.ExtractedLayers = mergeStringSlices(left.ExtractedLayers, right.ExtractedLayers)
	if keep.State == HistoryGroupOpen && drop.State == HistoryGroupClosed {
		keep.State = HistoryGroupClosed
	}
	if strings.TrimSpace(keep.CloseReason) == "" {
		keep.CloseReason = drop.CloseReason
	}
	return normalizeHistoryGroup(keep)
}

func preferHistoryGroup(left, right HistoryGroup) bool {
	if left.State != right.State {
		return left.State == HistoryGroupClosed
	}
	if len(left.ExtractedLayers) != len(right.ExtractedLayers) {
		return len(left.ExtractedLayers) > len(right.ExtractedLayers)
	}
	if strings.TrimSpace(left.SummaryHint) != "" && strings.TrimSpace(right.SummaryHint) == "" {
		return true
	}
	return left.GroupUUID < right.GroupUUID
}

func mergeStringSlices(left, right []string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	var out []string
	for _, item := range append(append([]string(nil), left...), right...) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func minTime(left, right time.Time) time.Time {
	if left.IsZero() {
		return right
	}
	if right.IsZero() {
		return left
	}
	if left.Before(right) {
		return left
	}
	return right
}

func maxTime(left, right time.Time) time.Time {
	if left.IsZero() {
		return right
	}
	if right.IsZero() {
		return left
	}
	if left.After(right) {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
