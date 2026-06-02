package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

type generatedMemory struct {
	Resident       string   `json:"resident"`
	Layer          string   `json:"layer"`
	DecisionAction string   `json:"decision_action"`
	ReasonCodes    []string `json:"reason_codes"`
	MemoryText     string   `json:"memory_text"`
	Accepted       bool     `json:"accepted"`
}

type memoryDoc struct {
	Path     string
	Modified time.Time
	Record   generatedMemory
}

type qualityReport struct {
	Docs                  int                `json:"docs"`
	Residents             []string           `json:"residents"`
	PersonaSeparation     int                `json:"persona_separation_score"`
	TemplateRigidity      int                `json:"template_rigidity_score"`
	DuplicatePressure     int                `json:"duplicate_pressure_score"`
	MemoryDensity         int                `json:"memory_density_score"`
	ResidentPhraseOverlap map[string][]string `json:"resident_phrase_overlap"`
	Findings              []string           `json:"findings"`
}

func main() {
	dir := flag.String("dir", "experiments/memory-runtime/output", "Memory runtime output directory")
	limit := flag.Int("limit", 12, "Maximum latest docs to inspect")
	flag.Parse()

	docs, err := loadLatestDocs(*dir, *limit)
	if err != nil {
		exitf("%v", err)
	}
	report := evaluateDocs(docs)
	out, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(out))
}

func loadLatestDocs(root string, limit int) ([]memoryDoc, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var docs []memoryDoc
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		record, err := loadDoc(path)
		if err != nil {
			continue
		}
		docs = append(docs, memoryDoc{
			Path:     path,
			Modified: info.ModTime(),
			Record:   record,
		})
	}
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Modified.After(docs[j].Modified)
	})
	if limit > 0 && len(docs) > limit {
		docs = docs[:limit]
	}
	return docs, nil
}

func loadDoc(path string) (generatedMemory, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return generatedMemory{}, err
	}
	content := string(raw)
	start := strings.Index(content, "```json")
	end := strings.LastIndex(content, "```")
	if start < 0 || end <= start {
		return generatedMemory{}, fmt.Errorf("no json block in %s", path)
	}
	block := strings.TrimSpace(content[start+7 : end])
	var record generatedMemory
	if err := json.Unmarshal([]byte(block), &record); err != nil {
		return generatedMemory{}, err
	}
	return record, nil
}

func evaluateDocs(docs []memoryDoc) qualityReport {
	report := qualityReport{
		Docs:                  len(docs),
		ResidentPhraseOverlap: map[string][]string{},
	}
	if len(docs) == 0 {
		report.Findings = append(report.Findings, "no docs loaded")
		return report
	}

	residentTexts := map[string][]string{}
	repeatedLines := map[string]int{}
	reasonCodeCounts := map[string]int{}
	totalNonEmptyLines := 0
	templateSignals := 0

	for _, doc := range docs {
		record := doc.Record
		if !slices.Contains(report.Residents, record.Resident) {
			report.Residents = append(report.Residents, record.Resident)
		}
		residentTexts[record.Resident] = append(residentTexts[record.Resident], record.MemoryText)

		lines := splitNonEmptyLines(record.MemoryText)
		totalNonEmptyLines += len(lines)
		for _, line := range lines {
			repeatedLines[line]++
			if strings.Contains(line, "why_this_is_worth_context_budget:") ||
				strings.Contains(line, "scope_boundary:") ||
				strings.Contains(line, "promote_or_decay:") {
				templateSignals++
			}
		}
		for _, code := range record.ReasonCodes {
			reasonCodeCounts[code]++
		}
	}

	sort.Strings(report.Residents)
	report.PersonaSeparation = personaSeparationScore(residentTexts)
	report.TemplateRigidity = templateRigidityScore(repeatedLines, templateSignals, len(docs))
	report.DuplicatePressure = duplicatePressureScore(repeatedLines)
	report.MemoryDensity = memoryDensityScore(totalNonEmptyLines, len(docs))
	report.ResidentPhraseOverlap = residentOverlap(residentTexts)

	if len(report.Residents) < 3 {
		report.Findings = append(report.Findings, "resident sample is incomplete; persona separation result is not yet meaningful")
	} else if report.PersonaSeparation < 50 {
		report.Findings = append(report.Findings, "resident outputs are still too close; persona separation is weak")
	}
	if report.TemplateRigidity > 60 {
		report.Findings = append(report.Findings, "template rigidity is high; outputs still carry strong structured-summary smell")
	}
	if report.DuplicatePressure > 45 {
		report.Findings = append(report.Findings, "duplicate pressure is high; memory store likely accumulates near-restatements")
	}
	if report.MemoryDensity < 45 {
		report.Findings = append(report.Findings, "memory density is low; outputs may still be verbose shells around thin signal")
	}
	if len(reasonCodeCounts) <= 4 {
		report.Findings = append(report.Findings, "reason code variety is still narrow; state judgments may be collapsing into repeated categories")
	}
	if len(report.Findings) == 0 {
		report.Findings = append(report.Findings, "no major static warning from the current sample")
	}
	return report
}

func splitNonEmptyLines(text string) []string {
	parts := strings.Split(text, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func personaSeparationScore(residentTexts map[string][]string) int {
	if len(residentTexts) < 2 {
		return 0
	}
	uniqueMarkers := map[string][]string{
		"jade":  {"engineering", "diagnostic", "reversibility"},
		"amber": {"handoff", "collaborator", "legible"},
		"onyx":  {"leverage", "bargaining", "false edge"},
	}
	score := 0
	for resident, texts := range residentTexts {
		blob := strings.ToLower(strings.Join(texts, "\n"))
		markers := uniqueMarkers[resident]
		local := 0
		for _, marker := range markers {
			if strings.Contains(blob, marker) {
				local += 30
			}
		}
		if local > 100 {
			local = 100
		}
		score += local
	}
	return score / len(residentTexts)
}

func templateRigidityScore(repeatedLines map[string]int, templateSignals, docs int) int {
	if docs == 0 {
		return 0
	}
	repeated := 0
	for _, count := range repeatedLines {
		if count >= 3 {
			repeated++
		}
	}
	score := repeated*10 + (templateSignals*100)/(docs*9)
	if score > 100 {
		return 100
	}
	return score
}

func duplicatePressureScore(repeatedLines map[string]int) int {
	score := 0
	for _, count := range repeatedLines {
		if count >= 2 {
			score += count * 4
		}
	}
	if score > 100 {
		return 100
	}
	return score
}

func memoryDensityScore(totalLines, docs int) int {
	if docs == 0 {
		return 0
	}
	avg := totalLines / docs
	switch {
	case avg >= 8 && avg <= 10:
		return 75
	case avg >= 6 && avg <= 12:
		return 60
	case avg >= 4:
		return 45
	default:
		return 25
	}
}

func residentOverlap(residentTexts map[string][]string) map[string][]string {
	phrases := map[string][]string{}
	for resident, texts := range residentTexts {
		blob := strings.ToLower(strings.Join(texts, "\n"))
		var overlaps []string
		for _, phrase := range []string{
			"false edge",
			"real edge",
			"scope boundary",
			"why_this_is_worth_context_budget",
			"promote_or_decay",
		} {
			if strings.Contains(blob, phrase) {
				overlaps = append(overlaps, phrase)
			}
		}
		phrases[resident] = overlaps
	}
	return phrases
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
