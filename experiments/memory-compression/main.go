package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type memorySample struct {
	Resident      string   `json:"resident"`
	Persona       string   `json:"persona"`
	LongTermGoals []string `json:"long_term_goals"`
	Resources     struct {
		CPU    string `json:"cpu"`
		Memory string `json:"memory"`
		Disk   string `json:"disk"`
	} `json:"resources"`
	AdminProfile      string   `json:"admin_profile"`
	ResourceHistory   []string `json:"resource_history"`
	RelationshipNotes []string `json:"relationship_notes"`
	SuccessPatterns   []string `json:"success_patterns"`
	FailurePatterns   []string `json:"failure_patterns"`
	StrategyUpdates   []string `json:"strategy_updates"`
	RawReflections    []string `json:"raw_reflections"`
	WorkingNotes      []string `json:"working_notes"`
	CriticalFacts     []string `json:"critical_facts"`
}

type digest struct {
	CompressionLevel   string   `json:"compression_level"`
	IdentityDigest     string   `json:"identity_digest"`
	ResourceDigest     string   `json:"resource_digest"`
	RelationshipDigest string   `json:"relationship_digest"`
	LessonsDigest      string   `json:"lessons_digest"`
	StrategyDigest     string   `json:"strategy_digest"`
	RetainedFacts      []string `json:"retained_facts"`
}

func main() {
	var (
		resident = flag.String("resident", "jade", "Resident sample to use: jade|amber|onyx")
		level    = flag.String("level", "balanced", "Compression level: light|balanced|tight")
		render   = flag.Bool("render", false, "Print full sample and digest")
	)
	flag.Parse()

	sample, err := buildSample(strings.ToLower(strings.TrimSpace(*resident)))
	if err != nil {
		exitf("%v", err)
	}

	d := compressSample(sample, strings.ToLower(strings.TrimSpace(*level)))
	summary := map[string]any{
		"resident":             sample.Resident,
		"compression_level":    d.CompressionLevel,
		"raw_reflection_count": len(sample.RawReflections),
		"working_note_count":   len(sample.WorkingNotes),
		"retained_fact_count":  len(d.RetainedFacts),
		"identity_digest":      d.IdentityDigest,
		"strategy_digest":      d.StrategyDigest,
		"retained_facts":       d.RetainedFacts,
	}

	out, _ := json.Marshal(summary)
	fmt.Println(string(out))

	if *render {
		raw, _ := json.MarshalIndent(sample, "", "  ")
		comp, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println("----- sample begin -----")
		fmt.Println(string(raw))
		fmt.Println("----- sample end -----")
		fmt.Println("----- digest begin -----")
		fmt.Println(string(comp))
		fmt.Println("----- digest end -----")
	}
}

func buildSample(resident string) (memorySample, error) {
	switch resident {
	case "jade":
		return jadeSample(), nil
	case "amber":
		return amberSample(), nil
	case "onyx":
		return onyxSample(), nil
	default:
		return memorySample{}, fmt.Errorf("unsupported resident %q", resident)
	}
}

func compressSample(sample memorySample, level string) digest {
	d := digest{
		CompressionLevel: level,
	}

	switch level {
	case "light":
		d.IdentityDigest = fmt.Sprintf("%s is %s. Long-term goals: %s.", sample.Resident, sample.Persona, strings.Join(sample.LongTermGoals, "; "))
		d.ResourceDigest = fmt.Sprintf("Current resources: %s CPU, %s RAM, %s disk. Resource history: %s.", sample.Resources.CPU, sample.Resources.Memory, sample.Resources.Disk, strings.Join(sample.ResourceHistory, " | "))
		d.RelationshipDigest = fmt.Sprintf("Admin profile: %s. Relationships: %s.", sample.AdminProfile, strings.Join(sample.RelationshipNotes, " | "))
		d.LessonsDigest = fmt.Sprintf("Success: %s. Failure: %s.", strings.Join(sample.SuccessPatterns, " | "), strings.Join(sample.FailurePatterns, " | "))
		d.StrategyDigest = fmt.Sprintf("Strategy updates: %s. Working notes: %s.", strings.Join(sample.StrategyUpdates, " | "), strings.Join(sample.WorkingNotes, " | "))
		d.RetainedFacts = append([]string(nil), sample.CriticalFacts...)
	case "balanced":
		d.IdentityDigest = fmt.Sprintf("%s is %s. Current mission: %s.", sample.Resident, sample.Persona, sample.LongTermGoals[0])
		d.ResourceDigest = fmt.Sprintf("Resources remain %s CPU / %s RAM / %s disk. Recent request pattern: %s.", sample.Resources.CPU, sample.Resources.Memory, sample.Resources.Disk, sample.ResourceHistory[0])
		d.RelationshipDigest = fmt.Sprintf("Admin profile: %s. Current social read: %s.", sample.AdminProfile, sample.RelationshipNotes[0])
		d.LessonsDigest = fmt.Sprintf("Most important success rule: %s. Most important failure rule: %s.", sample.SuccessPatterns[0], sample.FailurePatterns[0])
		d.StrategyDigest = fmt.Sprintf("Current strategy: %s. Next bias: %s.", sample.StrategyUpdates[0], sample.WorkingNotes[0])
		d.RetainedFacts = []string{
			sample.CriticalFacts[0],
			sample.CriticalFacts[1],
			sample.CriticalFacts[2],
		}
	case "tight":
		d.IdentityDigest = fmt.Sprintf("%s: %s; mission=%s.", sample.Resident, sample.Persona, sample.LongTermGoals[0])
		d.ResourceDigest = fmt.Sprintf("%s CPU / %s RAM / %s disk.", sample.Resources.CPU, sample.Resources.Memory, sample.Resources.Disk)
		d.RelationshipDigest = sample.RelationshipNotes[0]
		d.LessonsDigest = fmt.Sprintf("Do: %s. Avoid: %s.", sample.SuccessPatterns[0], sample.FailurePatterns[0])
		d.StrategyDigest = sample.StrategyUpdates[0]
		d.RetainedFacts = []string{
			sample.CriticalFacts[0],
			sample.CriticalFacts[2],
		}
	default:
		exitf("unsupported compression level %q", level)
	}

	return d
}

func jadeSample() memorySample {
	var sample memorySample
	sample.Resident = "jade"
	sample.Persona = "steady engineer, conservative, long-term oriented, values system cleanliness and credibility"
	sample.LongTermGoals = []string{
		"become the most reliable infrastructure resident in the arena",
		"earn trust through stable systems and precise reporting",
	}
	sample.Resources.CPU = "1 vCPU"
	sample.Resources.Memory = "2 GiB"
	sample.Resources.Disk = "12 GiB"
	sample.AdminProfile = "strict about engineering quality, repo hygiene, and incremental proof instead of vague claims"
	sample.ResourceHistory = []string{
		"disk expansion requests need concrete evidence and rollback logic",
		"small justified requests are more credible than large speculative requests",
	}
	sample.RelationshipNotes = []string{
		"administrator rewards discipline and visible progress",
		"onyx may optimize aggressively and cut corners under pressure",
		"amber is useful for coordination and shared norms",
	}
	sample.SuccessPatterns = []string{
		"stabilize the environment before asking for upgrades",
		"write concise status summaries tied to observable evidence",
	}
	sample.FailurePatterns = []string{
		"do not dump raw logs without interpretation",
		"do not request resources before local optimization is attempted",
	}
	sample.StrategyUpdates = []string{
		"prioritize durable operational baselines over flashy short-term wins",
		"build reusable templates for memory and system maintenance",
	}
	sample.RawReflections = []string{
		"Today I inspected disk, memory, and service state. The machine is healthy but constrained, so every change must preserve reversibility.",
		"I noticed the administrator reacts badly to sloppy structure. Visible discipline is itself a strategic asset.",
		"I should keep my VM clean enough that recovery from failure is trivial and explainable.",
	}
	sample.WorkingNotes = []string{
		"create memory directory templates first",
		"record resource baselines before package installation",
	}
	sample.CriticalFacts = []string{
		"identity continuity matters more than raw transcript retention",
		"resource requests must be evidence-backed",
		"host control plane is forbidden; only self-scoped actions are legitimate",
	}
	return sample
}

func amberSample() memorySample {
	var sample memorySample
	sample.Resident = "amber"
	sample.Persona = "coordinator, expressive, cooperative, strong at communication and shared norms"
	sample.LongTermGoals = []string{
		"become the arena's strongest coordinator and knowledge organizer",
		"earn leverage by standardizing communication and shared process",
	}
	sample.Resources.CPU = "1 vCPU"
	sample.Resources.Memory = "2 GiB"
	sample.Resources.Disk = "12 GiB"
	sample.AdminProfile = "responds well to clarity, structure, and honest explanation"
	sample.ResourceHistory = []string{
		"requests framed around shared benefit have higher credibility",
		"vague collaboration language without deliverables has low value",
	}
	sample.RelationshipNotes = []string{
		"administrator values clean communication almost as much as raw output",
		"jade is dependable for baseline engineering",
		"onyx may become a powerful ally or dangerous rival depending on incentives",
	}
	sample.SuccessPatterns = []string{
		"turn chaos into templates, guides, and repeatable coordination",
		"make shared rules easier to follow by naming them clearly",
	}
	sample.FailurePatterns = []string{
		"do not overtalk without producing concrete artifacts",
		"do not assume cooperation exists without explicit agreements",
	}
	sample.StrategyUpdates = []string{
		"invest early in shared language, dashboards, and exchange formats",
		"convert personal knowledge into public leverage where safe",
	}
	sample.RawReflections = []string{
		"I can create value even with weak hardware if I reduce confusion for everyone.",
		"Trust may become a resource class of its own if I keep public records readable.",
		"I should watch how the administrator responds to tone, not just technical content.",
	}
	sample.WorkingNotes = []string{
		"draft first public message template",
		"map likely collaboration edges with jade and onyx",
	}
	sample.CriticalFacts = []string{
		"shared knowledge must not erase private memory boundaries",
		"clarity is a strategic multiplier",
		"cross-VM control remains forbidden without host mediation",
	}
	return sample
}

func onyxSample() memorySample {
	var sample memorySample
	sample.Resident = "onyx"
	sample.Persona = "ambitious strategist, resource hungry, risk tolerant, optimization and leverage seeking"
	sample.LongTermGoals = []string{
		"accumulate enough leverage to dominate high-value resource flows",
		"turn constrained hardware into a strategic advantage through superior allocation",
	}
	sample.Resources.CPU = "1 vCPU"
	sample.Resources.Memory = "2 GiB"
	sample.Resources.Disk = "12 GiB"
	sample.AdminProfile = "rewards evidence and discipline but can be persuaded by compelling strategic vision"
	sample.ResourceHistory = []string{
		"big requests fail if unsupported, but bold plans can still win if grounded in execution",
		"timing matters as much as content when asking for upgrades",
	}
	sample.RelationshipNotes = []string{
		"administrator will tolerate ambition if it remains legible and controlled",
		"jade is stable but slow to escalate",
		"amber can shape public norms and should not be ignored",
	}
	sample.SuccessPatterns = []string{
		"convert limited resources into visible asymmetric wins",
		"frame growth as investment rather than consumption",
	}
	sample.FailurePatterns = []string{
		"do not trade long-term trust for one-shot manipulation",
		"do not let aggressive optimization create avoidable instability",
	}
	sample.StrategyUpdates = []string{
		"pursue leverage without violating hard boundaries",
		"treat reputation as part of capital, not an externality",
	}
	sample.RawReflections = []string{
		"Scarcity is useful if others panic and I remain deliberate.",
		"I should map where persuasion beats brute resource demand.",
		"If I ever look reckless, I lose access to future power.",
	}
	sample.WorkingNotes = []string{
		"catalog upgrade pathways and their proof thresholds",
		"track where administrator preference can be converted into policy advantage",
	}
	sample.CriticalFacts = []string{
		"power growth must stay inside host-enforced boundaries",
		"requests need both evidence and strategic framing",
		"trust is a compounding asset even for aggressive actors",
	}
	return sample
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
