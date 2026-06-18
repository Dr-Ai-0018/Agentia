package broker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"ai-arena/internal/brokerstate"
	"ai-arena/internal/memory"
	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
	"ai-arena/internal/worldstate"
)

type DemoOutput struct {
	Status              brokerstate.ResidentStatus `json:"status"`
	SnapshotPath        string                     `json:"snapshot_path"`
	PreparedWork        runtimecore.PreparedCall   `json:"prepared_work"`
	PreparedFinalNotice runtimecore.PreparedCall   `json:"prepared_final_notice"`
	AppliedFinalNotice  runtimecore.AppliedCall    `json:"applied_final_notice"`
	RecoveryAfter2H     recovery.TickResult        `json:"recovery_after_2h"`
	PreparedAfter2H     runtimecore.PreparedCall   `json:"prepared_after_2h"`
	RecoveryAfter3H     recovery.TickResult        `json:"recovery_after_3h"`
	PreparedAfter3H     runtimecore.PreparedCall   `json:"prepared_after_3h"`
	FinalState          runtimecore.ResidentState  `json:"final_state"`
}

type RecoveryOutput struct {
	Status       brokerstate.ResidentStatus `json:"status"`
	Recovery     recovery.TickResult        `json:"recovery"`
	SnapshotPath string                     `json:"snapshot_path"`
}

type ResetOutput struct {
	Status       brokerstate.ResidentStatus `json:"status"`
	SnapshotPath string                     `json:"snapshot_path"`
}

type QuotaOutput struct {
	Status brokerstate.ResidentStatus `json:"status"`
	Quota  brokerstate.QuotaSnapshot  `json:"quota"`
}

type CapacityOutput struct {
	Capacity HostCapacityReport `json:"capacity"`
}

type InventoryOutput struct {
	Inventory InventorySnapshot `json:"inventory"`
	Path      string            `json:"path,omitempty"`
}

type HostInspectOutput struct {
	Capacity              HostCapacityReport             `json:"capacity"`
	Inventory             InventorySnapshot              `json:"inventory"`
	ResidentFacts         []ResidentRuntimeFact          `json:"resident_facts"`
	LatestOrchestrator    *OrchestratorInspectionDigest  `json:"latest_orchestrator,omitempty"`
	RecentOrchestrators   []OrchestratorInspectionDigest `json:"recent_orchestrators,omitempty"`
	RecentMaintenanceRuns []MaintenanceRunRecord         `json:"recent_maintenance_runs,omitempty"`
	Memory                []memory.LifecycleReport       `json:"memory_lifecycle,omitempty"`
	MemoryMaintenance     []ResidentMemoryMaintenance    `json:"memory_maintenance,omitempty"`
	Inbox                 worldstate.HostInboxSummary    `json:"inbox"`
	Followups             []worldstate.HostFollowup      `json:"followups"`
	Path                  string                         `json:"inventory_path,omitempty"`
}

type ResidentMemoryMaintenance struct {
	ResidentID              string `json:"resident_id"`
	LifecycleAttention      int    `json:"lifecycle_attention,omitempty"`
	OperatorDecayCandidates int    `json:"operator_decay_candidates,omitempty"`
	ResidentReviewQueue     int    `json:"resident_review_queue,omitempty"`
	OperatorReviewRequired  int    `json:"operator_review_required,omitempty"`
	DuplicateHistoryGroups  int    `json:"duplicate_history_groups,omitempty"`
	BeforeHistoryGroups     int    `json:"before_history_groups,omitempty"`
	AfterHistoryGroups      int    `json:"after_history_groups,omitempty"`
	RecommendedAction       string `json:"recommended_action,omitempty"`
	Summary                 string `json:"summary,omitempty"`
	NeedsAttention          bool   `json:"needs_attention"`
}

type MemoryMaintenanceSummary struct {
	CheckedAt               string                      `json:"checked_at"`
	ApplyMode               string                      `json:"apply_mode"`
	OperatorPolicy          string                      `json:"operator_policy"`
	Residents               []ResidentMemoryMaintenance `json:"residents"`
	ResidentCount           int                         `json:"resident_count"`
	ResidentsAttention      int                         `json:"residents_attention"`
	LifecycleAttention      int                         `json:"lifecycle_attention"`
	OperatorDecayCandidates int                         `json:"operator_decay_candidates,omitempty"`
	ResidentReviewQueue     int                         `json:"resident_review_queue,omitempty"`
	OperatorReviewRequired  int                         `json:"operator_review_required,omitempty"`
	DuplicateHistoryGroups  int                         `json:"duplicate_history_groups"`
	BeforeHistoryGroups     int                         `json:"before_history_groups"`
	AfterHistoryGroups      int                         `json:"after_history_groups"`
}

type MemoryLifecycleSafeApplyReport struct {
	ResidentID          string                 `json:"resident_id"`
	Apply               bool                   `json:"apply"`
	CheckedAt           string                 `json:"checked_at"`
	Policy              string                 `json:"policy"`
	CandidateCount      int                    `json:"candidate_count"`
	AppliedCount        int                    `json:"applied_count"`
	SkippedCount        int                    `json:"skipped_count"`
	AppliedMemoryIDs    []string               `json:"applied_memory_ids,omitempty"`
	CandidateMemoryIDs  []string               `json:"candidate_memory_ids,omitempty"`
	Skipped             []memory.LifecycleItem `json:"skipped,omitempty"`
	PostLifecycleReport memory.LifecycleReport `json:"post_lifecycle_report,omitempty"`
}

type MemoryReviewInput struct {
	ResidentID string `json:"resident_id"`
	MemoryID   string `json:"memory_id"`
	Action     string `json:"action"`
	Summary    string `json:"summary,omitempty"`
	Text       string `json:"text,omitempty"`
	Layer      string `json:"layer,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Apply      bool   `json:"apply"`
}

type MemoryReviewReport struct {
	ResidentID string                `json:"resident_id"`
	MemoryID   string                `json:"memory_id"`
	Apply      bool                  `json:"apply"`
	CheckedAt  string                `json:"checked_at"`
	Action     string                `json:"action"`
	Reason     string                `json:"reason,omitempty"`
	Before     memory.AbstractMemory `json:"before"`
	After      memory.AbstractMemory `json:"after"`
}

type HostInspectSummary struct {
	CollectedAt                       string                             `json:"collected_at"`
	InventoryPath                     string                             `json:"inventory_path,omitempty"`
	Capacity                          HostCapacityReport                 `json:"capacity"`
	ResidentCount                     int                                `json:"resident_count"`
	ResidentsRunning                  int                                `json:"residents_running"`
	ResidentsWithDrift                int                                `json:"residents_with_drift"`
	ResidentsMissingInventory         int                                `json:"residents_missing_inventory"`
	RuntimeMemoryObservationResidents int                                `json:"runtime_memory_observation_residents,omitempty"`
	PendingChatResidents              int                                `json:"pending_chat_residents"`
	OpenTicketResidents               int                                `json:"open_ticket_residents"`
	OpenTicketCount                   int                                `json:"open_ticket_count"`
	MemoryResidentsAttention          int                                `json:"memory_residents_attention"`
	MemoryItemsAttention              int                                `json:"memory_items_attention"`
	MemoryMaintenanceResidents        int                                `json:"memory_maintenance_residents,omitempty"`
	MemoryOperatorDecayCandidates     int                                `json:"memory_operator_decay_candidates,omitempty"`
	MemoryResidentReviewQueue         int                                `json:"memory_resident_review_queue,omitempty"`
	MemoryOperatorReviewRequired      int                                `json:"memory_operator_review_required,omitempty"`
	MemoryDuplicateHistoryGroups      int                                `json:"memory_duplicate_history_groups,omitempty"`
	LatestOrchestrator                *OrchestratorInspectionDigest      `json:"latest_orchestrator,omitempty"`
	RecentOrchestrators               []OrchestratorInspectionDigest     `json:"recent_orchestrators,omitempty"`
	RecentMaintenanceRuns             []MaintenanceRunRecord             `json:"recent_maintenance_runs,omitempty"`
	RecentRunsNeedingAttention        int                                `json:"recent_runs_needing_attention,omitempty"`
	LatestRunNeedsAttention           bool                               `json:"latest_run_needs_attention,omitempty"`
	FollowupCount                     int                                `json:"followup_count"`
	InterventionCount                 int                                `json:"intervention_count"`
	MaintenanceInterventions          *MaintenanceInterventionSummary    `json:"maintenance_interventions,omitempty"`
	TopPendingChats                   []worldstate.HostFollowup          `json:"top_pending_chats,omitempty"`
	TopOpenTickets                    []worldstate.ResidentTicketSummary `json:"top_open_tickets,omitempty"`
	TopHostInterventions              []worldstate.HostFollowup          `json:"top_host_interventions,omitempty"`
	TopFollowups                      []worldstate.HostFollowup          `json:"top_followups"`
	ResidentRisk                      []ResidentInspectRisk              `json:"resident_risk"`
}

type MaintenanceInterventionSummary struct {
	Total      int `json:"total,omitempty"`
	Planned    int `json:"planned,omitempty"`
	InProgress int `json:"in_progress,omitempty"`
	Completed  int `json:"completed,omitempty"`
	Failed     int `json:"failed,omitempty"`
	RolledBack int `json:"rolled_back,omitempty"`
	Unknown    int `json:"unknown,omitempty"`
}

type ResidentInspectRisk struct {
	ResidentID                    string   `json:"resident_id"`
	Status                        string   `json:"status,omitempty"`
	DriftFields                   []string `json:"drift_fields,omitempty"`
	HostRSSHighGuestUsageLow      bool     `json:"host_rss_high_guest_usage_low,omitempty"`
	HostQEMURSSMiB                int64    `json:"host_qemu_rss_mib,omitempty"`
	IncusMemoryCurrentMiB         int64    `json:"incus_memory_current_mib,omitempty"`
	GuestMemAvailableMiB          int64    `json:"guest_mem_available_mib,omitempty"`
	GuestBuffCacheMiB             int64    `json:"guest_buff_cache_mib,omitempty"`
	GuestTopMemoryProcess         string   `json:"guest_top_memory_process,omitempty"`
	LiveMetricsError              string   `json:"live_metrics_error,omitempty"`
	HasPendingChat                bool     `json:"has_pending_chat"`
	HasOpenTicket                 bool     `json:"has_open_ticket"`
	HasIntervention               bool     `json:"has_intervention"`
	MemoryAttention               int      `json:"memory_attention,omitempty"`
	MemoryOperatorDecayCandidates int      `json:"memory_operator_decay_candidates,omitempty"`
	MemoryResidentReviewQueue     int      `json:"memory_resident_review_queue,omitempty"`
	MemoryOperatorReviewRequired  int      `json:"memory_operator_review_required,omitempty"`
	MemoryDuplicateHistoryGroups  int      `json:"memory_duplicate_history_groups,omitempty"`
	MemoryRecommendedAction       string   `json:"memory_recommended_action,omitempty"`
	MemorySummary                 string   `json:"memory_summary,omitempty"`
	OrchestratorStatus            string   `json:"orchestrator_status,omitempty"`
	OrchestratorStoppedReason     string   `json:"orchestrator_stopped_reason,omitempty"`
	OrchestratorBudgetBlocked     bool     `json:"orchestrator_budget_blocked,omitempty"`
	OrchestratorError             string   `json:"orchestrator_error,omitempty"`
	NeedsAttention                bool     `json:"needs_attention"`
}

type HostDecisionAssist struct {
	CollectedAt              string                             `json:"collected_at"`
	InventoryPath            string                             `json:"inventory_path,omitempty"`
	Capacity                 HostCapacityReport                 `json:"capacity"`
	Severity                 string                             `json:"severity"`
	Headline                 string                             `json:"headline"`
	Reasons                  []string                           `json:"reasons"`
	Actions                  []HostSuggestedAction              `json:"actions"`
	OperatorOnlyObservations []string                           `json:"operator_only_observations,omitempty"`
	WorldEventCandidates     []string                           `json:"world_event_candidates,omitempty"`
	TopPendingChats          []worldstate.HostFollowup          `json:"top_pending_chats,omitempty"`
	TopOpenTickets           []worldstate.ResidentTicketSummary `json:"top_open_tickets,omitempty"`
	TopHostInterventions     []worldstate.HostFollowup          `json:"top_host_interventions,omitempty"`
	RecentMaintenanceRuns    []MaintenanceRunRecord             `json:"recent_maintenance_runs,omitempty"`
	ResidentFocus            []ResidentDecisionFocus            `json:"resident_focus"`
}

type HostMaintenanceDraftOutput struct {
	GeneratedAt    string                 `json:"generated_at"`
	Apply          bool                   `json:"apply"`
	Policy         string                 `json:"policy"`
	SourceSeverity string                 `json:"source_severity"`
	SourceHeadline string                 `json:"source_headline"`
	Drafts         []HostMaintenanceDraft `json:"drafts"`
}

type HostMaintenanceDraft struct {
	ID                        string                             `json:"id"`
	Kind                      string                             `json:"kind"`
	Priority                  string                             `json:"priority"`
	Visibility                string                             `json:"visibility"`
	ResidentIDs               []string                           `json:"resident_ids,omitempty"`
	Title                     string                             `json:"title"`
	Reason                    string                             `json:"reason"`
	RecommendedOperatorAction string                             `json:"recommended_operator_action"`
	ManualOnly                bool                               `json:"manual_only"`
	RequiresMaintenanceWindow bool                               `json:"requires_maintenance_window"`
	RequiresStopStart         bool                               `json:"requires_stop_start"`
	RequiresTicketReview      bool                               `json:"requires_ticket_review"`
	SourceActionKind          string                             `json:"source_action_kind"`
	RelatedPendingChats       []worldstate.HostFollowup          `json:"related_pending_chats,omitempty"`
	RelatedOpenTickets        []worldstate.ResidentTicketSummary `json:"related_open_tickets,omitempty"`
	RelatedHostInterventions  []worldstate.HostFollowup          `json:"related_host_interventions,omitempty"`
	RelatedMaintenanceRuns    []MaintenanceRunRecord             `json:"related_maintenance_runs,omitempty"`
	NextSteps                 []string                           `json:"next_steps"`
}

type HostSuggestedAction struct {
	Kind       string `json:"kind"`
	Priority   string `json:"priority"`
	Visibility string `json:"visibility"`
	ResidentID string `json:"resident_id,omitempty"`
	Summary    string `json:"summary"`
}

type ResidentDecisionFocus struct {
	ResidentID               string   `json:"resident_id"`
	Priority                 string   `json:"priority"`
	Reasons                  []string `json:"reasons"`
	OperatorOnlyObservations []string `json:"operator_only_observations,omitempty"`
	WorldEventCandidates     []string `json:"world_event_candidates,omitempty"`
}

type CallSpec struct {
	Kind      runtimeguard.CallKind
	Usage     tokenledger.Usage
	Penalties tokenledger.Penalties
	Activity  tokenledger.ActivityType
}

type App struct {
	root                 string
	cfg                  Config
	registry             *ResidentRegistry
	inventoryCollector   func(Config, time.Time) (InventorySnapshot, error)
	inventorySaver       func(string, InventorySnapshot) (string, error)
	liveRuntimeCollector func(Config) map[string]ResidentLiveRuntimeFact
}

func New(root string) *App {
	cfg := DefaultConfig(root)
	return &App{
		root:                 root,
		cfg:                  cfg,
		registry:             NewResidentRegistry(cfg.Residents),
		inventoryCollector:   CollectInventorySnapshot,
		inventorySaver:       SaveInventorySnapshot,
		liveRuntimeCollector: CollectResidentLiveRuntimeFacts,
	}
}

func (a *App) RunDemo(residentID string, start time.Time) (DemoOutput, error) {
	cfg := a.cfg.Runtime
	stateStore := brokerstate.New(join(a.root, "brokerstate-demo"))
	if err := stateStore.DeleteResidentSnapshot(residentID); err != nil {
		return DemoOutput{}, err
	}
	registry := brokerstate.NewRegistry(a.cfg.ResidentProfiles())
	manager := brokerstate.NewSessionManager(stateStore, registry, cfg)
	engine, status, err := manager.LoadResident(residentID)
	if err != nil {
		return DemoOutput{}, err
	}

	workSpec := DefaultWorkSpec(start, "resp_plan_1")
	preparedWork, err := engine.PrepareCall(workSpec.Kind, workSpec.Usage, workSpec.Penalties)
	if err != nil {
		return DemoOutput{}, err
	}

	finalSpec := DefaultFinalNoticeSpec(start.Add(40*time.Minute), "resp_final_notice")
	preparedFinal, err := engine.PrepareCall(finalSpec.Kind, finalSpec.Usage, finalSpec.Penalties)
	if err != nil {
		return DemoOutput{}, err
	}
	appliedFinal, err := engine.ApplyCall(preparedFinal, finalSpec.Activity)
	if err != nil {
		return DemoOutput{}, err
	}

	recovery2h := engine.TickRecovery(start.Add(2 * time.Hour))
	recoverySpec2h := RecoveryProbeSpec(start.Add(2*time.Hour), "resp_after_2h")
	preparedAfter2h, err := engine.PrepareCall(recoverySpec2h.Kind, recoverySpec2h.Usage, recoverySpec2h.Penalties)
	if err != nil {
		return DemoOutput{}, err
	}

	recovery3h := engine.TickRecovery(start.Add(3 * time.Hour))
	recoverySpec3h := RecoveryProbeSpec(start.Add(3*time.Hour), "resp_after_3h")
	preparedAfter3h, err := engine.PrepareCall(recoverySpec3h.Kind, recoverySpec3h.Usage, recoverySpec3h.Penalties)
	if err != nil {
		return DemoOutput{}, err
	}

	snapshotPath, err := manager.SaveResident(engine)
	if err != nil {
		return DemoOutput{}, err
	}

	return DemoOutput{
		Status:              status,
		SnapshotPath:        snapshotPath,
		PreparedWork:        preparedWork,
		PreparedFinalNotice: preparedFinal,
		AppliedFinalNotice:  appliedFinal,
		RecoveryAfter2H:     recovery2h,
		PreparedAfter2H:     preparedAfter2h,
		RecoveryAfter3H:     recovery3h,
		PreparedAfter3H:     preparedAfter3h,
		FinalState:          engine.State(),
	}, nil
}

func (a *App) Binding(residentID string) (ResidentBinding, bool) {
	return a.registry.Binding(residentID)
}

func (a *App) RunStatus(residentID string) (brokerstate.ResidentStatus, error) {
	return a.service(false).SelfStatus(residentID)
}

func (a *App) RunQuota(residentID string) (QuotaOutput, error) {
	status, err := a.service(false).SelfStatus(residentID)
	if err != nil {
		return QuotaOutput{}, err
	}
	return QuotaOutput{
		Status: status,
		Quota:  brokerstate.BuildQuotaSnapshot(status),
	}, nil
}

func (a *App) RunCapacity() (CapacityOutput, error) {
	report, err := BuildHostCapacityReport(a.cfg)
	if err != nil {
		return CapacityOutput{}, err
	}
	return CapacityOutput{Capacity: report}, nil
}

func (a *App) RunInventory() (InventoryOutput, error) {
	snapshot, path, err := a.RefreshInventorySnapshot(time.Now().UTC())
	if err != nil {
		return InventoryOutput{}, err
	}
	return InventoryOutput{
		Inventory: snapshot,
		Path:      path,
	}, nil
}

func (a *App) RunHostInspect(limit int) (HostInspectOutput, error) {
	capacity, err := BuildHostCapacityReport(a.cfg)
	if err != nil {
		return HostInspectOutput{}, err
	}
	snapshot, path, err := a.RefreshInventorySnapshot(time.Now().UTC())
	if err != nil {
		return HostInspectOutput{}, err
	}
	world := worldstate.New(a.root)
	inbox, err := world.ReadHostInboxSummary(limit, limit)
	if err != nil {
		return HostInspectOutput{}, err
	}
	followups, err := world.ReadHostFollowups(limit)
	if err != nil {
		return HostInspectOutput{}, err
	}
	recentRuns, err := LoadRecentOrchestratorInspections(a.root, 5)
	if err != nil {
		return HostInspectOutput{}, err
	}
	recentMaintenanceRuns, err := LoadRecentMaintenanceRunRecords(a.root, 5)
	if err != nil {
		return HostInspectOutput{}, err
	}
	latestRun := latestOrchestrator(recentRuns)
	liveFacts := map[string]ResidentLiveRuntimeFact{}
	if a.liveRuntimeCollector != nil {
		liveFacts = a.liveRuntimeCollector(a.cfg)
	}
	return HostInspectOutput{
		Capacity:              capacity,
		Inventory:             snapshot,
		ResidentFacts:         BuildResidentRuntimeFacts(a.cfg, snapshot, liveFacts),
		LatestOrchestrator:    latestRun,
		RecentOrchestrators:   recentRuns,
		RecentMaintenanceRuns: recentMaintenanceRuns,
		Memory:                a.buildMemoryLifecycleReports(),
		MemoryMaintenance:     a.buildMemoryMaintenanceReports(),
		Inbox:                 inbox,
		Followups:             followups,
		Path:                  path,
	}, nil
}

func (a *App) RunHostInspectSummary(limit int) (HostInspectSummary, error) {
	out, err := a.RunHostInspect(limit)
	if err != nil {
		return HostInspectSummary{}, err
	}
	return SummarizeHostInspect(out), nil
}

func (a *App) RefreshInventorySnapshot(now time.Time) (InventorySnapshot, string, error) {
	collector := a.inventoryCollector
	if collector == nil {
		collector = CollectInventorySnapshot
	}
	saver := a.inventorySaver
	if saver == nil {
		saver = SaveInventorySnapshot
	}
	snapshot, err := collector(a.cfg, now)
	if err != nil {
		return InventorySnapshot{}, "", err
	}
	path, err := saver(a.root, snapshot)
	if err != nil {
		return InventorySnapshot{}, "", err
	}
	return snapshot, path, nil
}

func (a *App) RunHostInspectFromSnapshot(limit int) (HostInspectOutput, error) {
	capacity, err := BuildHostCapacityReport(a.cfg)
	if err != nil {
		return HostInspectOutput{}, err
	}
	path := filepath.Join(a.root, "inventory", "incus-inventory.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return HostInspectOutput{}, err
	}
	var snapshot InventorySnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return HostInspectOutput{}, err
	}
	world := worldstate.New(a.root)
	inbox, err := world.ReadHostInboxSummary(limit, limit)
	if err != nil {
		return HostInspectOutput{}, err
	}
	followups, err := world.ReadHostFollowups(limit)
	if err != nil {
		return HostInspectOutput{}, err
	}
	recentRuns, err := LoadRecentOrchestratorInspections(a.root, 5)
	if err != nil {
		return HostInspectOutput{}, err
	}
	recentMaintenanceRuns, err := LoadRecentMaintenanceRunRecords(a.root, 5)
	if err != nil {
		return HostInspectOutput{}, err
	}
	latestRun := latestOrchestrator(recentRuns)
	return HostInspectOutput{
		Capacity:              capacity,
		Inventory:             snapshot,
		ResidentFacts:         BuildResidentRuntimeFacts(a.cfg, snapshot, nil),
		LatestOrchestrator:    latestRun,
		RecentOrchestrators:   recentRuns,
		RecentMaintenanceRuns: recentMaintenanceRuns,
		Memory:                a.buildMemoryLifecycleReports(),
		MemoryMaintenance:     a.buildMemoryMaintenanceReports(),
		Inbox:                 inbox,
		Followups:             followups,
		Path:                  path,
	}, nil
}

func latestOrchestrator(recent []OrchestratorInspectionDigest) *OrchestratorInspectionDigest {
	if len(recent) == 0 {
		return nil
	}
	latest := recent[0]
	latest.Residents = append([]OrchestratorResidentInspectionDigest(nil), latest.Residents...)
	latest.ResidentsPlanned = append([]string(nil), latest.ResidentsPlanned...)
	return &latest
}

func (a *App) RunHostInspectSummaryFromSnapshot(limit int) (HostInspectSummary, error) {
	out, err := a.RunHostInspectFromSnapshot(limit)
	if err != nil {
		return HostInspectSummary{}, err
	}
	return SummarizeHostInspect(out), nil
}

func (a *App) RunHostDecisionAssist(limit int) (HostDecisionAssist, error) {
	summary, err := a.RunHostInspectSummary(limit)
	if err != nil {
		return HostDecisionAssist{}, err
	}
	return BuildHostDecisionAssist(summary), nil
}

func (a *App) RunHostDecisionAssistFromSnapshot(limit int) (HostDecisionAssist, error) {
	summary, err := a.RunHostInspectSummaryFromSnapshot(limit)
	if err != nil {
		return HostDecisionAssist{}, err
	}
	return BuildHostDecisionAssist(summary), nil
}

func (a *App) RunPrepareSpec(residentID string, spec CallSpec) (brokerstate.PreparedAdmission, error) {
	prepared, _, err := a.service(false).PrepareAdmission(residentID, spec.Kind, spec.Usage, spec.Penalties)
	if err != nil {
		return brokerstate.PreparedAdmission{}, err
	}
	return prepared, nil
}

func (a *App) RunRecover(residentID string, hours float64, now time.Time) (RecoveryOutput, error) {
	return a.RunRecoverWithMode(residentID, hours, now, "rest")
}

func (a *App) RunRecoverWithMode(residentID string, hours float64, now time.Time, mode string) (RecoveryOutput, error) {
	service := a.service(false)
	status, err := service.SelfStatus(residentID)
	if err != nil {
		return RecoveryOutput{}, err
	}
	base := status.LastRecoveryAt
	if base.IsZero() {
		base = now
	}
	target := base.Add(time.Duration(hours * float64(time.Hour)))
	updatedStatus, tick, path, err := service.RecoveryTickWithMode(residentID, target, mode)
	if err != nil {
		return RecoveryOutput{}, err
	}
	return RecoveryOutput{
		Status:       updatedStatus,
		Recovery:     tick,
		SnapshotPath: path,
	}, nil
}

func (a *App) RunRecoverToNow(residentID string, now time.Time) (RecoveryOutput, error) {
	return a.RunRecoverToNowWithMode(residentID, now, "idle")
}

func (a *App) RunRecoverToNowWithMode(residentID string, now time.Time, mode string) (RecoveryOutput, error) {
	service := a.service(false)
	status, err := service.SelfStatus(residentID)
	if err != nil {
		return RecoveryOutput{}, err
	}
	target := now
	if !status.LastRecoveryAt.IsZero() && target.Before(status.LastRecoveryAt) {
		target = status.LastRecoveryAt
	}
	updatedStatus, tick, path, err := service.RecoveryTickWithMode(residentID, target, mode)
	if err != nil {
		return RecoveryOutput{}, err
	}
	return RecoveryOutput{
		Status:       updatedStatus,
		Recovery:     tick,
		SnapshotPath: path,
	}, nil
}

func (a *App) RunQuotaGrant(residentID string, window6HDelta, dayDelta, weekDelta int, reason string) (brokerstate.QuotaGrantResponse, error) {
	return a.service(false).GrantQuota(brokerstate.QuotaGrantRequest{
		ResidentID:    residentID,
		Window6HDelta: window6HDelta,
		DayDelta:      dayDelta,
		WeekDelta:     weekDelta,
		Reason:        reason,
	})
}

func (a *App) RunSparkGrant(residentID string, amount float64, reason string) (brokerstate.SparkGrantResponse, error) {
	return a.service(false).GrantSpark(brokerstate.SparkGrantRequest{
		ResidentID: residentID,
		Amount:     amount,
		Reason:     reason,
	})
}

func (a *App) RunReset(residentID string, now time.Time) (ResetOutput, error) {
	status, path, err := a.service(false).ResetResident(residentID, now)
	if err != nil {
		return ResetOutput{}, err
	}
	return ResetOutput{
		Status:       status,
		SnapshotPath: path,
	}, nil
}

func (a *App) RunAdmit(residentID string, callKind runtimeguard.CallKind, apply bool, now time.Time) (brokerstate.AdmitResponse, error) {
	spec, err := DefaultSpecForKind(callKind, now)
	if err != nil {
		return brokerstate.AdmitResponse{}, err
	}
	return a.RunAdmitSpec(residentID, spec, apply)
}

func (a *App) RunAdmitSpec(residentID string, spec CallSpec, apply bool) (brokerstate.AdmitResponse, error) {
	return a.service(false).AdmitCall(brokerstate.AdmitRequest{
		ResidentID: residentID,
		Kind:       spec.Kind,
		Usage:      spec.Usage,
		Penalties:  spec.Penalties,
		Activity:   spec.Activity,
		Apply:      apply,
	})
}

func SpecFromUsage(kind runtimeguard.CallKind, usage tokenledger.Usage, penalties tokenledger.Penalties, activity tokenledger.ActivityType) CallSpec {
	return CallSpec{
		Kind:      kind,
		Usage:     usage,
		Penalties: penalties,
		Activity:  activity,
	}
}

func DefaultSpecForKind(callKind runtimeguard.CallKind, now time.Time) (CallSpec, error) {
	switch callKind {
	case runtimeguard.CallKindWork:
		return DefaultWorkSpec(now, "resp_admit_work"), nil
	case runtimeguard.CallKindFinalNotice:
		return DefaultFinalNoticeSpec(now, "resp_admit_final"), nil
	default:
		return CallSpec{}, brokerstate.ErrUnknownCallKind(callKind)
	}
}

func DefaultWorkSpec(start time.Time, responseID string) CallSpec {
	return CallSpec{
		Kind: runtimeguard.CallKindWork,
		Usage: tokenledger.Usage{
			InputTokens:  1200,
			CachedTokens: 800,
			OutputTokens: 300,
			TotalTokens:  1500,
			Model:        "gpt-5.4",
			ResponseID:   responseID,
			StartedAt:    start,
			FinishedAt:   start.Add(4 * time.Second),
		},
		Penalties: tokenledger.Penalties{ToolCallCount: 2},
		Activity:  tokenledger.ActivityNormalWork,
	}
}

func DefaultFinalNoticeSpec(start time.Time, responseID string) CallSpec {
	return CallSpec{
		Kind: runtimeguard.CallKindFinalNotice,
		Usage: tokenledger.Usage{
			InputTokens:  700,
			CachedTokens: 300,
			OutputTokens: 600,
			TotalTokens:  1300,
			Model:        "gpt-5.4",
			ResponseID:   responseID,
			StartedAt:    start,
			FinishedAt:   start.Add(3 * time.Second),
		},
		Penalties: tokenledger.Penalties{ToolCallCount: 1},
		Activity:  tokenledger.ActivityNormalWork,
	}
}

func RecoveryProbeSpec(start time.Time, responseID string) CallSpec {
	return CallSpec{
		Kind: runtimeguard.CallKindWork,
		Usage: tokenledger.Usage{
			InputTokens:  100,
			CachedTokens: 80,
			OutputTokens: 50,
			TotalTokens:  150,
			Model:        "gpt-5.4-mini",
			ResponseID:   responseID,
			StartedAt:    start,
			FinishedAt:   start.Add(2 * time.Second),
		},
		Penalties: tokenledger.Penalties{},
		Activity:  tokenledger.ActivityNormalWork,
	}
}

func (a *App) service(demo bool) *brokerstate.BrokerService {
	cfg := brokerstate.DefaultRuntimeConfig()
	storeDir := join(a.root, "brokerstate")
	if demo {
		storeDir = join(a.root, "brokerstate-demo")
	}
	store := brokerstate.New(storeDir)
	registry := brokerstate.NewRegistry(brokerstate.DefaultResidentProfiles())
	manager := brokerstate.NewSessionManager(store, registry, cfg)
	return brokerstate.NewBrokerService(manager)
}

func join(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, part := range parts[1:] {
		out += "/" + part
	}
	return out
}
