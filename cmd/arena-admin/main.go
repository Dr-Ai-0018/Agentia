package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type auditResult struct {
	Mode               string   `json:"mode"`
	Apply              bool     `json:"apply"`
	ScannedFiles       int      `json:"scanned_files"`
	AffectedFiles      int      `json:"affected_files"`
	RemovedMessageIDs  []string `json:"removed_message_ids,omitempty"`
	KeptMessageIDs     []string `json:"kept_message_ids,omitempty"`
	BackupFilesCreated []string `json:"backup_files_created,omitempty"`
}

type legacyWorldMessage struct {
	ID                string `json:"id"`
	Direction         string `json:"direction"`
	AutoFeedbackForID string `json:"auto_feedback_for_id,omitempty"`
}

func main() {
	mode := flag.String("mode", "world-audit", "Mode: world-audit|world-cleanup")
	apply := flag.Bool("apply", false, "Apply changes for cleanup mode")
	root := flag.String("root", ".agents", "Arena state root")
	flag.Parse()

	switch *mode {
	case "world-audit":
		out, err := runWorldCleanup(*root, false)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "world-cleanup":
		out, err := runWorldCleanup(*root, *apply)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	default:
		exitf("unknown mode: %s", *mode)
	}
}

func runWorldCleanup(root string, apply bool) (auditResult, error) {
	files, err := filepath.Glob(filepath.Join(root, "world", "messages", "*.jsonl"))
	if err != nil {
		return auditResult{}, err
	}

	result := auditResult{
		Mode:         "world-cleanup",
		Apply:        apply,
		ScannedFiles: len(files),
	}

	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return auditResult{}, err
		}

		lines := []string{}
		changed := false

		scanner := bufio.NewScanner(strings.NewReader(string(raw)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var msg legacyWorldMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				return auditResult{}, fmt.Errorf("decode %s: %w", file, err)
			}

			if isLegacyChatAutoFeedback(msg) {
				changed = true
				result.RemovedMessageIDs = append(result.RemovedMessageIDs, msg.ID)
				continue
			}

			result.KeptMessageIDs = append(result.KeptMessageIDs, msg.ID)
			lines = append(lines, line)
		}
		if err := scanner.Err(); err != nil {
			return auditResult{}, err
		}

		if !changed {
			continue
		}

		result.AffectedFiles++
		if !apply {
			continue
		}

		backup := file + ".bak"
		if err := os.WriteFile(backup, raw, 0o644); err != nil {
			return auditResult{}, err
		}
		result.BackupFilesCreated = append(result.BackupFilesCreated, backup)

		content := ""
		if len(lines) > 0 {
			content = strings.Join(lines, "\n") + "\n"
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			return auditResult{}, err
		}
	}

	return result, nil
}

func isLegacyChatAutoFeedback(msg legacyWorldMessage) bool {
	return msg.Direction == "system_to_resident" && strings.TrimSpace(msg.AutoFeedbackForID) != ""
}

func printJSON(v any) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		exitf("marshal json: %v", err)
	}
	fmt.Println(string(raw))
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
