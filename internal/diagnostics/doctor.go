package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ai-arena/internal/memory"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/worldstate"
)

type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type Finding struct {
	Severity Severity `json:"severity"`
	Scope    string   `json:"scope"`
	Path     string   `json:"path,omitempty"`
	Message  string   `json:"message"`
}

type ResidentHealth struct {
	Resident            string `json:"resident"`
	HasBrokerSnapshot   bool   `json:"has_broker_snapshot"`
	SnapshotRevision    uint64 `json:"snapshot_revision,omitempty"`
	HasMemoryBundle     bool   `json:"has_memory_bundle"`
	HistoryGroupCount   int    `json:"history_group_count"`
	AbstractMemoryCount int    `json:"abstract_memory_count"`
	PendingChatCount    int    `json:"pending_chat_count"`
	OpenTicketCount     int    `json:"open_ticket_count"`
}

type CountSummary struct {
	Residents       int `json:"residents"`
	BrokerSnapshots int `json:"broker_snapshots"`
	MemoryBundles   int `json:"memory_bundles"`
	MessageFiles    int `json:"message_files"`
	MessageRecords  int `json:"message_records"`
	TicketFiles     int `json:"ticket_files"`
	OpenTickets     int `json:"open_tickets"`
	PendingChats    int `json:"pending_chats"`
	AuditFiles      int `json:"audit_files"`
	PublicHistories int `json:"public_history_files"`
}

type DoctorReport struct {
	OK        bool             `json:"ok"`
	Root      string           `json:"root"`
	Counts    CountSummary     `json:"counts"`
	Residents []ResidentHealth `json:"residents"`
	Findings  []Finding        `json:"findings"`
}

func RunDoctor(root string) (DoctorReport, error) {
	report := DoctorReport{
		OK:   true,
		Root: root,
	}

	residentMap := map[string]*ResidentHealth{}
	addResident := func(name string) *ResidentHealth {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil
		}
		if existing, ok := residentMap[name]; ok {
			return existing
		}
		item := &ResidentHealth{Resident: name}
		residentMap[name] = item
		return item
	}
	addFinding := func(severity Severity, scope, path, message string) {
		report.Findings = append(report.Findings, Finding{
			Severity: severity,
			Scope:    scope,
			Path:     path,
			Message:  message,
		})
		if severity == SeverityError {
			report.OK = false
		}
	}

	if err := scanBrokerSnapshots(root, addResident, &report.Counts, addFinding); err != nil {
		return DoctorReport{}, err
	}
	if err := scanMemoryBundles(root, addResident, &report.Counts, addFinding); err != nil {
		return DoctorReport{}, err
	}
	if err := scanWorld(root, addResident, &report.Counts, addFinding); err != nil {
		return DoctorReport{}, err
	}
	if err := scanArtifacts(root, &report.Counts, addFinding); err != nil {
		return DoctorReport{}, err
	}

	for _, resident := range residentMap {
		if !resident.HasBrokerSnapshot {
			addFinding(SeverityWarn, "brokerstate", "", "resident is missing broker snapshot: "+resident.Resident)
		}
		if !resident.HasMemoryBundle {
			addFinding(SeverityWarn, "memory", "", "resident is missing memory bundle: "+resident.Resident)
		}
	}

	report.Residents = make([]ResidentHealth, 0, len(residentMap))
	for _, resident := range residentMap {
		report.Residents = append(report.Residents, *resident)
	}
	sort.Slice(report.Residents, func(i, j int) bool {
		return report.Residents[i].Resident < report.Residents[j].Resident
	})
	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Severity == report.Findings[j].Severity {
			if report.Findings[i].Scope == report.Findings[j].Scope {
				return report.Findings[i].Message < report.Findings[j].Message
			}
			return report.Findings[i].Scope < report.Findings[j].Scope
		}
		return severityRank(report.Findings[i].Severity) > severityRank(report.Findings[j].Severity)
	})
	report.Counts.Residents = len(report.Residents)
	return report, nil
}

func severityRank(s Severity) int {
	switch s {
	case SeverityError:
		return 3
	case SeverityWarn:
		return 2
	default:
		return 1
	}
}

func scanBrokerSnapshots(root string, addResident func(string) *ResidentHealth, counts *CountSummary, addFinding func(Severity, string, string, string)) error {
	files, err := filepath.Glob(filepath.Join(root, "brokerstate", "*", "runtime-state.json"))
	if err != nil {
		return err
	}
	counts.BrokerSnapshots = len(files)
	for _, path := range files {
		resident := filepath.Base(filepath.Dir(path))
		item := addResident(resident)
		raw, err := os.ReadFile(path)
		if err != nil {
			addFinding(SeverityError, "brokerstate", path, "cannot read broker snapshot: "+err.Error())
			continue
		}
		var snapshot runtimecore.Snapshot
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			addFinding(SeverityError, "brokerstate", path, "cannot parse broker snapshot: "+err.Error())
			continue
		}
		item.HasBrokerSnapshot = true
		item.SnapshotRevision = snapshot.Revision
		if strings.TrimSpace(snapshot.Version) == "" {
			addFinding(SeverityWarn, "brokerstate", path, "broker snapshot has empty version")
		}
		if strings.TrimSpace(snapshot.State.ResidentID) == "" {
			addFinding(SeverityWarn, "brokerstate", path, "broker snapshot has empty resident id")
		} else if snapshot.State.ResidentID != resident {
			addFinding(SeverityError, "brokerstate", path, "broker snapshot resident id does not match directory name")
		}
	}
	return nil
}

func scanMemoryBundles(root string, addResident func(string) *ResidentHealth, counts *CountSummary, addFinding func(Severity, string, string, string)) error {
	files, err := filepath.Glob(filepath.Join(root, "memory", "*.json"))
	if err != nil {
		return err
	}
	counts.MemoryBundles = len(files)
	for _, path := range files {
		resident := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		item := addResident(resident)
		raw, err := os.ReadFile(path)
		if err != nil {
			addFinding(SeverityError, "memory", path, "cannot read memory bundle: "+err.Error())
			continue
		}
		var bundle memory.ResidentMemoryBundle
		if err := json.Unmarshal(raw, &bundle); err != nil {
			var legacy []memory.AbstractMemory
			if legacyErr := json.Unmarshal(raw, &legacy); legacyErr != nil {
				addFinding(SeverityError, "memory", path, "cannot parse memory bundle: "+err.Error())
				continue
			}
			bundle.AbstractMemories = legacy
			addFinding(SeverityWarn, "memory", path, "memory file is still using legacy array format")
		}
		item.HasMemoryBundle = true
		item.HistoryGroupCount = len(bundle.HistoryGroups)
		item.AbstractMemoryCount = len(bundle.AbstractMemories)
		if duplicates := countDuplicateHistoryGroupSignatures(bundle.HistoryGroups); duplicates > 0 {
			addFinding(SeverityWarn, "memory", path, "memory bundle has duplicate history groups; run memory-compact for resident: "+resident)
		}
	}
	return nil
}

func countDuplicateHistoryGroupSignatures(groups []memory.HistoryGroup) int {
	seen := map[string]struct{}{}
	duplicates := 0
	for _, group := range groups {
		signature := strings.Join(group.RawEventRefs, "\n")
		signature = strings.TrimSpace(signature)
		if signature == "" {
			continue
		}
		if _, ok := seen[signature]; ok {
			duplicates++
			continue
		}
		seen[signature] = struct{}{}
	}
	return duplicates
}

func scanWorld(root string, addResident func(string) *ResidentHealth, counts *CountSummary, addFinding func(Severity, string, string, string)) error {
	messageFiles, err := filepath.Glob(filepath.Join(root, "world", "messages", "*.jsonl"))
	if err != nil {
		return err
	}
	counts.MessageFiles = len(messageFiles)

	worldStore := worldstate.New(root)
	for _, issue := range worldStore.ScanMessageFiles() {
		msg := issue.Message
		if issue.Line > 0 {
			msg = msg + " at line " + itoa(issue.Line)
		}
		addFinding(SeverityError, "world", issue.Path, msg)
	}

	repliedIDs := map[string]struct{}{}
	residentToMessageIDs := map[string][]string{}
	for _, path := range messageFiles {
		raw, err := os.ReadFile(path)
		if err != nil {
			addFinding(SeverityError, "world", path, "cannot read message file: "+err.Error())
			continue
		}
		lines := strings.Split(string(raw), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var msg worldstate.Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			counts.MessageRecords++
			item := addResident(msg.Resident)
			if item == nil {
				addFinding(SeverityWarn, "world", path, "message record missing resident id")
				continue
			}
			if msg.Direction == worldstate.DirectionResidentToChenglin {
				residentToMessageIDs[msg.Resident] = append(residentToMessageIDs[msg.Resident], msg.ID)
			}
			if msg.Direction == worldstate.DirectionChenglinToResident && strings.TrimSpace(msg.ReplyToID) != "" {
				repliedIDs[msg.ReplyToID] = struct{}{}
			}
		}
	}
	for resident, ids := range residentToMessageIDs {
		item := addResident(resident)
		for _, id := range ids {
			if _, ok := repliedIDs[id]; !ok {
				item.PendingChatCount++
				counts.PendingChats++
			}
		}
	}

	ticketFiles, err := filepath.Glob(filepath.Join(root, "world", "tickets", "*.json"))
	if err != nil {
		return err
	}
	counts.TicketFiles = len(ticketFiles)
	for _, path := range ticketFiles {
		raw, err := os.ReadFile(path)
		if err != nil {
			addFinding(SeverityError, "world", path, "cannot read ticket file: "+err.Error())
			continue
		}
		var ticket worldstate.Ticket
		if err := json.Unmarshal(raw, &ticket); err != nil {
			addFinding(SeverityError, "world", path, "cannot parse ticket file: "+err.Error())
			continue
		}
		item := addResident(ticket.Resident)
		if item == nil {
			addFinding(SeverityWarn, "world", path, "ticket missing resident id")
			continue
		}
		if ticket.Status == worldstate.TicketStatusOpen {
			item.OpenTicketCount++
			counts.OpenTickets++
		}
	}
	return nil
}

func scanArtifacts(root string, counts *CountSummary, addFinding func(Severity, string, string, string)) error {
	auditFiles, err := filepath.Glob(filepath.Join(root, "audit", "*.jsonl"))
	if err != nil {
		return err
	}
	counts.AuditFiles = len(auditFiles)

	historyFiles, err := filepath.Glob(filepath.Join(root, "world", "public-history-*.jsonl"))
	if err != nil {
		return err
	}
	counts.PublicHistories = len(historyFiles)

	backupFiles, err := filepath.Glob(filepath.Join(root, "world", "messages", "*.bak"))
	if err != nil {
		return err
	}
	for _, path := range backupFiles {
		addFinding(SeverityInfo, "world", path, "backup message file exists")
	}

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			addFinding(SeverityWarn, "filesystem", path, "cannot walk path: "+err.Error())
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".tmp") {
			addFinding(SeverityWarn, "filesystem", path, "temporary file should be cleaned up")
		}
		return nil
	})
	return nil
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
