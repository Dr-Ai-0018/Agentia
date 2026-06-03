package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type guestSample struct {
	Instance     string         `json:"instance"`
	TimestampUTC string         `json:"timestamp_utc"`
	Host         hostView       `json:"host"`
	Guest        guestView      `json:"guest"`
	Assessment   assessmentView `json:"assessment"`
}

type hostView struct {
	CPUQuota    string `json:"cpu_quota"`
	MemoryLimit string `json:"memory_limit"`
	DiskSize    string `json:"disk_size"`
	Status      string `json:"status"`
	SnapshotCnt int    `json:"snapshot_count"`
}

type guestView struct {
	CPU          cpuView          `json:"cpu"`
	Memory       memoryView       `json:"memory"`
	Disk         diskView         `json:"disk"`
	Directories  []directoryView  `json:"directories"`
	SystemdUnits []systemdUnit    `json:"systemd_units"`
}

type cpuView struct {
	CPUs      int    `json:"cpus"`
	ModelName string `json:"model_name"`
	LoadAvg   string `json:"load_avg"`
}

type memoryView struct {
	TotalMiB     int `json:"total_mib"`
	UsedMiB      int `json:"used_mib"`
	AvailableMiB int `json:"available_mib"`
}

type diskView struct {
	RootSizeGiB int `json:"root_size_gib"`
	RootUsedGiB int `json:"root_used_gib"`
	RootUsePct  int `json:"root_use_pct"`
}

type directoryView struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Entries int    `json:"entries"`
}

type systemdUnit struct {
	Name   string `json:"name"`
	Active string `json:"active"`
	Sub    string `json:"sub"`
}

type assessmentView struct {
	NeedsMemoryRequest bool     `json:"needs_memory_request"`
	NeedsDiskRequest   bool     `json:"needs_disk_request"`
	Signals            []string `json:"signals"`
	Summary            string   `json:"summary"`
}

func main() {
	instance := flag.String("instance", "jade", "Incus VM name")
	outDir := flag.String("out-dir", "", "Optional directory to write sample json")
	flag.Parse()

	sample, err := collect(strings.TrimSpace(*instance))
	if err != nil {
		exitf("%v", err)
	}

	raw, _ := json.MarshalIndent(sample, "", "  ")
	fmt.Println(string(raw))

	if strings.TrimSpace(*outDir) != "" {
		if err := writeSample(*outDir, sample); err != nil {
			exitf("%v", err)
		}
	}
}

func collect(instance string) (guestSample, error) {
	hostInfo, err := runIncus("info", instance)
	if err != nil {
		return guestSample{}, err
	}
	hostConfig, err := runIncus("config", "show", instance, "--expanded")
	if err != nil {
		return guestSample{}, err
	}
	snapshotsJSON, err := runIncus("snapshot", "list", instance, "--format", "json")
	if err != nil {
		return guestSample{}, err
	}

	host := hostView{
		CPUQuota:    extractAfter(hostConfig, "limits.cpu:"),
		MemoryLimit: extractAfter(hostConfig, "limits.memory:"),
		DiskSize:    extractAfter(hostConfig, "size:"),
		Status:      extractAfter(hostInfo, "Status:"),
		SnapshotCnt: countSnapshots(snapshotsJSON),
	}

	guest := guestView{
		CPU: cpuView{
			CPUs:      parseInt(execGuest(instance, `nproc 2>/dev/null || echo 0`)),
			ModelName: strings.TrimSpace(execGuest(instance, `awk -F: 'tolower($1) ~ /model name/ {gsub(/^[ \t]+/, "", $2); print $2; exit}' /proc/cpuinfo`)),
			LoadAvg:   strings.TrimSpace(execGuest(instance, `cut -d' ' -f1-3 /proc/loadavg`)),
		},
		Memory: parseMemory(execGuest(instance, `free -m | awk 'NR==2 {print $2 " " $3 " " $7}'`)),
		Disk:   parseDisk(execGuest(instance, `df -BG / | awk 'NR==2 {gsub("G","",$2); gsub("G","",$3); gsub("%","",$5); print $2 " " $3 " " $5}'`)),
		Directories: []directoryView{
			readDirSample(instance, "/root"),
			readDirSample(instance, "/var/log"),
			readDirSample(instance, "/tmp"),
			readDirSample(instance, "/run/incus_agent"),
		},
		SystemdUnits: parseSystemdUnits(execGuest(instance, `systemctl list-units --type=service --state=running --no-legend --no-pager | head -n 8`)),
	}

	assessment := assess(host, guest)
	return guestSample{
		Instance:     instance,
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		Host:         host,
		Guest:        guest,
		Assessment:   assessment,
	}, nil
}

func assess(host hostView, guest guestView) assessmentView {
	signals := make([]string, 0, 8)
	needsMemory := false
	needsDisk := false

	if guest.Memory.AvailableMiB > 0 && guest.Memory.AvailableMiB < 256 {
		needsMemory = true
		signals = append(signals, "available memory below 256 MiB")
	}
	if guest.Disk.RootUsePct >= 85 {
		needsDisk = true
		signals = append(signals, "root filesystem usage at or above 85%")
	}
	if guest.CPU.LoadAvg != "" && guest.CPU.CPUs > 0 {
		parts := strings.Fields(guest.CPU.LoadAvg)
		if len(parts) > 0 {
			load1, _ := strconv.ParseFloat(parts[0], 64)
			if load1 > float64(guest.CPU.CPUs)*1.2 {
				signals = append(signals, "cpu load is consistently above current cpu quota")
			}
		}
	}
	if host.Status != "RUNNING" {
		signals = append(signals, "host reports instance not running")
	}
	if len(signals) == 0 {
		signals = append(signals, "no immediate resource pressure detected")
	}

	summary := fmt.Sprintf(
		"%s has %d MiB available memory, %d%% root disk usage, CPU quota %s, memory limit %s, disk size %s.",
		host.Status,
		guest.Memory.AvailableMiB,
		guest.Disk.RootUsePct,
		blankFallback(host.CPUQuota, "unset"),
		blankFallback(host.MemoryLimit, "unset"),
		blankFallback(host.DiskSize, "unset"),
	)

	return assessmentView{
		NeedsMemoryRequest: needsMemory,
		NeedsDiskRequest:   needsDisk,
		Signals:            signals,
		Summary:            summary,
	}
}

func writeSample(outDir string, sample guestSample) error {
	runDir := filepath.Join(outDir, sample.Instance+"-"+time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	raw, _ := json.MarshalIndent(sample, "", "  ")
	return os.WriteFile(filepath.Join(runDir, "sample.json"), raw, 0o644)
}

func runIncus(args ...string) (string, error) {
	cmd := exec.Command("incus", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("incus %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func execGuest(instance, script string) string {
	cmd := exec.Command("incus", "exec", instance, "--", "bash", "-lc", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readDirSample(instance, path string) directoryView {
	out := execGuest(instance, fmt.Sprintf(`if [ -d %q ]; then count=$(find %q -mindepth 1 -maxdepth 1 | wc -l); echo "yes $count"; else echo "no 0"; fi`, path, path))
	fields := strings.Fields(out)
	view := directoryView{Path: path}
	if len(fields) >= 2 {
		view.Exists = fields[0] == "yes"
		view.Entries = parseInt(fields[1])
	}
	return view
}

func parseMemory(text string) memoryView {
	fields := strings.Fields(text)
	if len(fields) < 3 {
		return memoryView{}
	}
	return memoryView{
		TotalMiB:     parseInt(fields[0]),
		UsedMiB:      parseInt(fields[1]),
		AvailableMiB: parseInt(fields[2]),
	}
}

func parseDisk(text string) diskView {
	fields := strings.Fields(text)
	if len(fields) < 3 {
		return diskView{}
	}
	return diskView{
		RootSizeGiB: parseInt(fields[0]),
		RootUsedGiB: parseInt(fields[1]),
		RootUsePct:  parseInt(fields[2]),
	}
}

func parseSystemdUnits(text string) []systemdUnit {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	units := make([]systemdUnit, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 4 {
			continue
		}
		units = append(units, systemdUnit{
			Name:   fields[0],
			Active: fields[2],
			Sub:    fields[3],
		})
	}
	return units
}

func countSnapshots(raw string) int {
	var snapshots []map[string]any
	if err := json.Unmarshal([]byte(raw), &snapshots); err != nil {
		return 0
	}
	return len(snapshots)
}

func extractAfter(text, marker string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, marker) {
			return strings.TrimSpace(strings.TrimPrefix(line, marker))
		}
	}
	return ""
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func blankFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
