package memory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	Visibility      Visibility     `json:"visibility,omitempty"`
	Tags            []string       `json:"tags,omitempty"`
	Governance      GovernanceMeta `json:"governance,omitempty"`
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

type GovernanceMeta struct {
	Quality       string    `json:"quality,omitempty"`
	ReviewState   string    `json:"review_state,omitempty"`
	ReviewReason  string    `json:"review_reason,omitempty"`
	FlaggedBy     string    `json:"flagged_by,omitempty"`
	FlaggedAt     time.Time `json:"flagged_at,omitempty"`
	ResidentMay   []string  `json:"resident_may,omitempty"`
	HostMay       []string  `json:"host_may,omitempty"`
	ProtectedFrom []string  `json:"protected_from,omitempty"`
}

type SnapshotEntry struct {
	ID              string     `json:"id"`
	Layer           Layer      `json:"layer"`
	DecisionAction  Action     `json:"decision_action"`
	Summary         string     `json:"summary"`
	ResidentText    string     `json:"resident_text,omitempty"`
	Visibility      Visibility `json:"visibility,omitempty"`
	MemoryKind      string     `json:"memory_kind,omitempty"`
	Salience        int        `json:"salience,omitempty"`
	EmotionTone     string     `json:"emotion_tone,omitempty"`
	TimeScope       string     `json:"time_scope,omitempty"`
	RetentionIntent string     `json:"retention_intent,omitempty"`
	DropCondition   string     `json:"drop_condition,omitempty"`
}

type ResidentMemoryBundle struct {
	HistoryGroups    []HistoryGroup   `json:"history_groups"`
	AbstractMemories []AbstractMemory `json:"abstract_memories"`
}

type CompactReport struct {
	Resident              string              `json:"resident"`
	Apply                 bool                `json:"apply"`
	BeforeHistoryGroups   int                 `json:"before_history_groups"`
	AfterHistoryGroups    int                 `json:"after_history_groups"`
	BeforeSourceGroupRefs int                 `json:"before_source_group_refs"`
	AfterSourceGroupRefs  int                 `json:"after_source_group_refs"`
	Changed               bool                `json:"changed"`
	MergeGroups           []CompactMergeGroup `json:"merge_groups,omitempty"`
}

type CompactMergeGroup struct {
	Signature        string   `json:"signature,omitempty"`
	KeepGroupUUID    string   `json:"keep_group_uuid"`
	DropGroupUUIDs   []string `json:"drop_group_uuids,omitempty"`
	GroupUUIDs       []string `json:"group_uuids"`
	SummaryHint      string   `json:"summary_hint,omitempty"`
	RawEventRefCount int      `json:"raw_event_ref_count"`
}

type LifecycleItem struct {
	ID                        string       `json:"id"`
	Layer                     Layer        `json:"layer"`
	Domain                    Domain       `json:"domain,omitempty"`
	Visibility                Visibility   `json:"visibility,omitempty"`
	Status                    RecordStatus `json:"status"`
	Action                    Action       `json:"action"`
	TargetLayer               Layer        `json:"target_layer"`
	ReasonCodes               []string     `json:"reason_codes,omitempty"`
	Summary                   string       `json:"summary,omitempty"`
	RecommendedOperatorAction string       `json:"recommended_operator_action,omitempty"`
	ReviewAt                  time.Time    `json:"review_at,omitempty"`
	ExpiresAt                 time.Time    `json:"expires_at,omitempty"`
	HardExpiresAt             time.Time    `json:"hard_expires_at,omitempty"`
	HardExpired               bool         `json:"hard_expired"`
	NeedsAttention            bool         `json:"needs_attention"`
}

type LifecycleReport struct {
	Resident       string          `json:"resident"`
	Apply          bool            `json:"apply"`
	CheckedAt      time.Time       `json:"checked_at"`
	Total          int             `json:"total"`
	NeedsAttention int             `json:"needs_attention"`
	ActionCounts   map[Action]int  `json:"action_counts"`
	Items          []LifecycleItem `json:"items"`
}

type Store interface {
	ListAbstractMemories(resident string) ([]AbstractMemory, error)
	UpsertAbstractMemory(record AbstractMemory) error
	GetAbstractMemory(resident, id string) (AbstractMemory, bool, error)
	ReviewAbstractMemory(resident, id string, now time.Time, review MemoryReviewRequest) (AbstractMemory, error)
	ListHistoryGroups(resident string) ([]HistoryGroup, error)
	UpsertHistoryGroup(group HistoryGroup) error
	CompactResident(resident string) error
}

type MemoryReviewRequest struct {
	Action       Action
	NewSummary   string
	NewText      string
	TargetLayer  Layer
	ReasonNote   string
	ResidentNote string
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

func (s *MemoryStore) ReviewAbstractMemory(resident, id string, now time.Time, review MemoryReviewRequest) (AbstractMemory, error) {
	list := s.abstractMemories[resident]
	for i := range list {
		if list[i].ID != id {
			continue
		}
		updated, err := applyMemoryReview(now, list[i], review)
		if err != nil {
			return AbstractMemory{}, err
		}
		list[i] = updated
		s.abstractMemories[resident] = list
		return updated, nil
	}
	return AbstractMemory{}, errors.New("memory record not found")
}

func (s *MemoryStore) UpsertAbstractMemory(record AbstractMemory) error {
	record = NormalizeAbstractMemory(record)
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
		record = NormalizeAbstractMemory(record)
		if record.Status == StatusDeleted {
			continue
		}
		entries = append(entries, SnapshotEntry{
			ID:              record.ID,
			Layer:           record.Layer,
			DecisionAction:  record.DecisionAction,
			Summary:         record.EffectiveSummary(),
			ResidentText:    record.ResidentText,
			Visibility:      record.Visibility,
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

func NormalizeAbstractMemory(record AbstractMemory) AbstractMemory {
	record.Visibility = NormalizeVisibility(record.Visibility)
	return record
}

func NormalizeVisibility(visibility Visibility) Visibility {
	switch visibility {
	case VisibilityPublic,
		VisibilityRelationship,
		VisibilityResidentPrivate,
		VisibilityPrivateJournal,
		VisibilitySystemAudit,
		VisibilityOperatorObservation:
		return visibility
	case "":
		return VisibilityResidentPrivate
	default:
		return VisibilityResidentPrivate
	}
}

func ResidentDigestVisible(record AbstractMemory) bool {
	switch NormalizeVisibility(record.Visibility) {
	case VisibilityPublic, VisibilityRelationship, VisibilityResidentPrivate:
		return true
	default:
		return false
	}
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
	for i := range records {
		records[i] = NormalizeAbstractMemory(records[i])
	}
	sortAbstractMemories(records)
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

func (s *FileStore) ReviewAbstractMemory(resident, id string, now time.Time, review MemoryReviewRequest) (AbstractMemory, error) {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return AbstractMemory{}, err
	}
	bundle, err := s.loadBundle(resident)
	if err != nil {
		return AbstractMemory{}, err
	}
	for i := range bundle.AbstractMemories {
		if bundle.AbstractMemories[i].ID != id {
			continue
		}
		updated, err := applyMemoryReview(now, bundle.AbstractMemories[i], review)
		if err != nil {
			return AbstractMemory{}, err
		}
		bundle.AbstractMemories[i] = updated
		if err := s.writeBundle(resident, bundle); err != nil {
			return AbstractMemory{}, err
		}
		return updated, nil
	}
	return AbstractMemory{}, errors.New("memory record not found")
}

func (s *FileStore) UpsertAbstractMemory(record AbstractMemory) error {
	record = NormalizeAbstractMemory(record)
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
	_, err := s.CompactResidentWithReport(resident, true)
	return err
}

func (s *FileStore) CompactResidentWithReport(resident string, apply bool) (CompactReport, error) {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return CompactReport{}, err
	}
	bundle, err := s.loadBundle(resident)
	if err != nil {
		return CompactReport{}, err
	}
	groups := append([]HistoryGroup(nil), bundle.HistoryGroups...)
	for i := range groups {
		groups[i] = normalizeHistoryGroup(groups[i])
	}
	compactedGroups, groupIDMap := compactHistoryGroups(groups)
	compactedRecords := remapAbstractMemoryGroups(bundle.AbstractMemories, groupIDMap)
	report := CompactReport{
		Resident:              strings.TrimSpace(resident),
		Apply:                 apply,
		BeforeHistoryGroups:   len(groups),
		AfterHistoryGroups:    len(compactedGroups),
		BeforeSourceGroupRefs: countSourceGroupRefs(bundle.AbstractMemories),
		AfterSourceGroupRefs:  countSourceGroupRefs(compactedRecords),
		Changed:               !reflect.DeepEqual(groups, compactedGroups) || !reflect.DeepEqual(bundle.AbstractMemories, compactedRecords),
		MergeGroups:           buildCompactMergeGroups(groups, groupIDMap),
	}
	if !apply {
		return report, nil
	}
	bundle.HistoryGroups = compactedGroups
	bundle.AbstractMemories = compactedRecords
	return report, s.writeBundle(resident, bundle)
}

func (s *FileStore) LifecycleReport(resident string, now time.Time, policy Policy) (LifecycleReport, error) {
	return s.LifecycleReportWithApply(resident, now, policy, false)
}

func (s *FileStore) LifecycleReportWithApply(resident string, now time.Time, policy Policy, apply bool) (LifecycleReport, error) {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return LifecycleReport{}, err
	}
	bundle, err := s.loadBundle(resident)
	if err != nil {
		return LifecycleReport{}, err
	}
	records := append([]AbstractMemory(nil), bundle.AbstractMemories...)
	sortAbstractMemories(records)
	report := buildLifecycleReport(resident, records, now, policy, apply)
	if !apply || report.NeedsAttention == 0 {
		return report, nil
	}
	for i := range bundle.AbstractMemories {
		if bundle.AbstractMemories[i].Status == StatusDeleted {
			continue
		}
		decision := lifecycleDecision(bundle.AbstractMemories[i], report.CheckedAt, policy)
		if decision.Action == ActionRetain {
			continue
		}
		bundle.AbstractMemories[i].Record = ApplyDecision(report.CheckedAt, bundle.AbstractMemories[i].Record, decision)
		if bundle.AbstractMemories[i].Governance.ReviewState == "" && decision.Action == ActionReview {
			bundle.AbstractMemories[i].Governance.ReviewState = "needs_resident_review"
			bundle.AbstractMemories[i].Governance.ReviewReason = strings.Join(decision.ReasonCodes, ",")
		}
	}
	if err := s.writeBundle(resident, bundle); err != nil {
		return LifecycleReport{}, err
	}
	return report, nil
}

func buildLifecycleReport(resident string, records []AbstractMemory, now time.Time, policy Policy, apply bool) LifecycleReport {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if policy.InstantTTL == 0 && policy.ShortTTL == 0 && policy.LongTTL == 0 && policy.PermanentReview == 0 {
		policy = DefaultPolicy()
	}
	report := LifecycleReport{
		Resident:     strings.TrimSpace(resident),
		Apply:        apply,
		CheckedAt:    now.UTC(),
		Total:        len(records),
		ActionCounts: map[Action]int{},
		Items:        make([]LifecycleItem, 0, len(records)),
	}
	for _, record := range records {
		if record.Status == StatusDeleted {
			continue
		}
		decision := lifecycleDecision(record, now, policy)
		hardExpired := !record.HardExpiresAt.IsZero() && !record.HardExpiresAt.After(now)
		needsAttention := decision.Action != ActionRetain || hardExpired || dueAt(record.ReviewAt, now) || dueAt(record.ExpiresAt, now)
		item := LifecycleItem{
			ID:                        record.ID,
			Layer:                     record.Layer,
			Domain:                    record.Domain,
			Visibility:                NormalizeVisibility(record.Visibility),
			Status:                    record.Status,
			Action:                    decision.Action,
			TargetLayer:               decision.TargetLayer,
			ReasonCodes:               append([]string(nil), decision.ReasonCodes...),
			Summary:                   record.EffectiveSummary(),
			RecommendedOperatorAction: recommendLifecycleOperatorAction(record, decision, hardExpired),
			ReviewAt:                  record.ReviewAt,
			ExpiresAt:                 record.ExpiresAt,
			HardExpiresAt:             record.HardExpiresAt,
			HardExpired:               hardExpired,
			NeedsAttention:            needsAttention,
		}
		report.ActionCounts[decision.Action]++
		if needsAttention {
			report.NeedsAttention++
		}
		report.Items = append(report.Items, item)
	}
	return report
}

func recommendLifecycleOperatorAction(record AbstractMemory, decision Decision, hardExpired bool) string {
	if decision.Action == ActionRetain && !hardExpired {
		return "retain"
	}
	summary := strings.ToLower(record.EffectiveSummary())
	switch record.Domain {
	case DomainIdentity, DomainRelationships, DomainRules, DomainHistory:
		return "review_for_promotion_or_rewrite"
	}
	for _, marker := range []string{
		"chenglin",
		"persistence",
		"world-facing thread",
		"ticket",
		"relationship",
		"policy",
		"continuity",
	} {
		if strings.Contains(summary, marker) {
			return "review_for_promotion_or_rewrite"
		}
	}
	if hardExpired || decision.Action == ActionDecay {
		return "decay_ok_after_spot_check"
	}
	return "review"
}

func lifecycleDecision(record AbstractMemory, now time.Time, policy Policy) Decision {
	touch := record.LastAccessedAt
	if touch.IsZero() {
		touch = record.UpdatedAt
	}
	if touch.IsZero() {
		touch = record.CreatedAt
	}
	created := record.CreatedAt
	if created.IsZero() {
		created = touch
	}
	return policy.EvaluateDecay(record.Layer, EventSignal{
		AgeSinceTouch:    nonNegativeDuration(now.Sub(touch)),
		AgeSinceCreation: nonNegativeDuration(now.Sub(created)),
		UserPinned:       record.Pinned,
	})
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
	for i := range bundle.AbstractMemories {
		bundle.AbstractMemories[i] = NormalizeAbstractMemory(bundle.AbstractMemories[i])
	}
	return bundle, nil
}

func (s *FileStore) writeBundle(resident string, bundle ResidentMemoryBundle) error {
	raw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteResidentBundle(s.residentPath(resident), raw, 0o644)
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
	if decision.Action == ActionRetain || decision.Action == ActionUpdate || decision.Action == ActionPromote || decision.Action == ActionDecay || decision.Action == ActionReview {
		record.LastAccessedAt = now
	}
	if decision.TTL > 0 {
		record.ExpiresAt = now.Add(decision.TTL)
	}
	if decision.Action == ActionDecay {
		record.ReviewAfter = time.Time{}
		record.ReviewAt = time.Time{}
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

func applyMemoryReview(now time.Time, record AbstractMemory, review MemoryReviewRequest) (AbstractMemory, error) {
	switch review.Action {
	case ActionRetain:
		record.Record = ApplyDecision(now, record.Record, Decision{
			Action:      ActionRetain,
			TargetLayer: defaultReviewTargetLayer(record.Layer, review.TargetLayer),
			ReviewAfter: 24 * time.Hour,
			ReasonCodes: []string{"resident_review_keep"},
		})
		record.Governance.ReviewState = "resolved"
		record.Governance.ReviewReason = strings.TrimSpace(review.ReasonNote)
	case ActionUpdate:
		record.Record = ApplyDecision(now, record.Record, Decision{
			Action:      ActionUpdate,
			TargetLayer: defaultReviewTargetLayer(record.Layer, review.TargetLayer),
			TTL:         ttlForLayer(defaultReviewTargetLayer(record.Layer, review.TargetLayer)),
			ReviewAfter: 24 * time.Hour,
			ReasonCodes: []string{"resident_review_rewrite"},
		})
		if strings.TrimSpace(review.NewSummary) != "" {
			record.Summary = strings.TrimSpace(review.NewSummary)
		}
		if strings.TrimSpace(review.NewText) != "" {
			record.ResidentText = strings.TrimSpace(review.NewText)
		} else if strings.TrimSpace(review.NewSummary) != "" {
			record.ResidentText = strings.TrimSpace(review.NewSummary)
		}
		record.Governance.ReviewState = "resolved"
		record.Governance.ReviewReason = strings.TrimSpace(review.ReasonNote)
	case ActionSummarize:
		record.Record = ApplyDecision(now, record.Record, Decision{
			Action:      ActionUpdate,
			TargetLayer: defaultReviewTargetLayer(record.Layer, review.TargetLayer),
			TTL:         ttlForLayer(defaultReviewTargetLayer(record.Layer, review.TargetLayer)),
			ReviewAfter: 24 * time.Hour,
			ReasonCodes: []string{"resident_review_compress"},
		})
		if strings.TrimSpace(review.NewSummary) != "" {
			record.Summary = strings.TrimSpace(review.NewSummary)
		}
		if strings.TrimSpace(review.NewText) != "" {
			record.ResidentText = strings.TrimSpace(review.NewText)
		} else if strings.TrimSpace(review.NewSummary) != "" {
			record.ResidentText = strings.TrimSpace(review.NewSummary)
		}
		record.Governance.ReviewState = "resolved"
		record.Governance.ReviewReason = strings.TrimSpace(review.ReasonNote)
	case ActionDecay:
		target := defaultDemotionTarget(record.Layer, review.TargetLayer)
		record.Record = ApplyDecision(now, record.Record, Decision{
			Action:      ActionDecay,
			TargetLayer: target,
			TTL:         ttlForLayer(target),
			ReasonCodes: []string{"resident_review_demote"},
		})
		record.Governance.ReviewState = "resolved"
		record.Governance.ReviewReason = strings.TrimSpace(review.ReasonNote)
	case ActionDelete:
		record.Record = ApplyDecision(now, record.Record, Decision{
			Action:      ActionDelete,
			TargetLayer: record.Layer,
			ReasonCodes: []string{"resident_review_delete"},
		})
		record.Governance.ReviewState = "resolved"
		record.Governance.ReviewReason = strings.TrimSpace(review.ReasonNote)
	default:
		return AbstractMemory{}, errors.New("unsupported resident memory review action")
	}
	if note := strings.TrimSpace(review.ResidentNote); note != "" {
		record.Tags = append(record.Tags, "resident_reviewed")
	}
	record.Governance.FlaggedBy = "resident"
	record.Governance.FlaggedAt = now
	record.Governance.Quality = fallbackReviewQuality(record.Governance.Quality, record)
	record.Tags = uniqueTrimmed(record.Tags)
	return record, nil
}

func fallbackReviewQuality(current string, record AbstractMemory) string {
	current = strings.TrimSpace(current)
	if current != "" && current != "low" {
		return current
	}
	if strings.TrimSpace(record.Summary) == "" && strings.TrimSpace(record.ResidentText) == "" {
		return "low"
	}
	if len(strings.TrimSpace(record.Summary)) < 24 && len(strings.TrimSpace(record.ResidentText)) < 24 {
		return "thin"
	}
	return "resident_reviewed"
}

func defaultReviewTargetLayer(current, requested Layer) Layer {
	if strings.TrimSpace(string(requested)) != "" {
		return requested
	}
	return current
}

func defaultDemotionTarget(current, requested Layer) Layer {
	if strings.TrimSpace(string(requested)) != "" {
		return requested
	}
	switch current {
	case LayerPermanent:
		return LayerLong
	case LayerLong:
		return LayerShort
	case LayerShort:
		return LayerInstant
	default:
		return LayerInstant
	}
}

func ttlForLayer(layer Layer) time.Duration {
	switch layer {
	case LayerInstant:
		return DefaultPolicy().InstantTTL
	case LayerShort:
		return DefaultPolicy().ShortTTL
	case LayerLong:
		return DefaultPolicy().LongTTL
	default:
		return 0
	}
}

func uniqueTrimmed(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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

func buildCompactMergeGroups(groups []HistoryGroup, groupIDMap map[string]string) []CompactMergeGroup {
	byKeep := map[string][]HistoryGroup{}
	keepByID := map[string]HistoryGroup{}
	for _, group := range groups {
		group = normalizeHistoryGroup(group)
		keepID := group.GroupUUID
		if replacement, ok := groupIDMap[group.GroupUUID]; ok && strings.TrimSpace(replacement) != "" {
			keepID = replacement
		}
		byKeep[keepID] = append(byKeep[keepID], group)
	}
	out := make([]CompactMergeGroup, 0, len(byKeep))
	for keepID, members := range byKeep {
		if len(members) <= 1 {
			continue
		}
		sort.Slice(members, func(i, j int) bool {
			return members[i].GroupUUID < members[j].GroupUUID
		})
		for _, member := range members {
			if member.GroupUUID == keepID {
				keepByID[keepID] = member
				break
			}
		}
		keep := keepByID[keepID]
		if strings.TrimSpace(keep.GroupUUID) == "" {
			keep = members[0]
		}
		item := CompactMergeGroup{
			Signature:        historyGroupSignature(keep),
			KeepGroupUUID:    keepID,
			GroupUUIDs:       make([]string, 0, len(members)),
			SummaryHint:      keep.SummaryHint,
			RawEventRefCount: len(keep.RawEventRefs),
		}
		for _, member := range members {
			item.GroupUUIDs = append(item.GroupUUIDs, member.GroupUUID)
			if member.GroupUUID != keepID {
				item.DropGroupUUIDs = append(item.DropGroupUUIDs, member.GroupUUID)
			}
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].KeepGroupUUID < out[j].KeepGroupUUID
	})
	return out
}

func remapAbstractMemoryGroups(records []AbstractMemory, groupIDMap map[string]string) []AbstractMemory {
	out := make([]AbstractMemory, 0, len(records))
	for _, record := range records {
		record.SourceGroupIDs = remapGroupIDs(record.SourceGroupIDs, groupIDMap)
		out = append(out, record)
	}
	return out
}

func countSourceGroupRefs(records []AbstractMemory) int {
	total := 0
	for _, record := range records {
		total += len(record.SourceGroupIDs)
	}
	return total
}

func dueAt(value, now time.Time) bool {
	return !value.IsZero() && !value.After(now)
}

func nonNegativeDuration(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	return value
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

func sortAbstractMemories(records []AbstractMemory) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].UpdatedAt.Equal(records[j].UpdatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
}

func atomicWriteResidentBundle(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
