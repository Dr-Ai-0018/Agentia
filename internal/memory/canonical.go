package memory

import "time"

type Event struct {
	Time       time.Time
	Category   string
	Importance int
	Summary    string
}

type CanonicalMemory struct {
	Domain          Domain
	Trigger         string
	MistakenBelief  string
	CorrectedBelief string
	ActionBoundary  string
	PreservedCost   string
	ScopeLimit      string
	DecisionImpact  float64
	Confidence      float64
	ReasonCodes     []string
}

func (cm CanonicalMemory) ToEventSignal() EventSignal {
	signal := EventSignal{
		Domain:         cm.Domain,
		DecisionImpact: cm.DecisionImpact,
		Confidence:     cm.Confidence,
	}

	switch cm.Domain {
	case DomainIdentity:
		signal.IdentityWeight = 0.9
	case DomainRules:
		signal.RuleWeight = 0.85
	case DomainResources:
		signal.ResourceWeight = 0.85
	case DomainRelationships:
		signal.RelationshipWeight = 0.8
	case DomainLessons:
		signal.RuleWeight = 0.55
	}

	for _, code := range cm.ReasonCodes {
		switch code {
		case "repeated_failure":
			signal.Recurrence++
			signal.ImpactRounds += 2
			signal.RuleWeight += 0.15
		case "recovery_after_shift":
			signal.ImpactRounds += 1
			signal.DecisionImpact += 0.1
		case "admin_feedback":
			signal.RelationshipWeight += 0.15
			signal.RuleWeight += 0.1
		case "resource_signal":
			signal.ResourceWeight += 0.1
		}
	}

	if signal.DecisionImpact > 1 {
		signal.DecisionImpact = 1
	}
	if signal.RuleWeight > 1 {
		signal.RuleWeight = 1
	}
	if signal.ResourceWeight > 1 {
		signal.ResourceWeight = 1
	}
	if signal.RelationshipWeight > 1 {
		signal.RelationshipWeight = 1
	}
	return signal
}

func DistillEvents(events []Event) CanonicalMemory {
	cm := CanonicalMemory{
		Domain:         DomainLessons,
		DecisionImpact: 0.4,
		Confidence:     0.7,
	}

	var sawFailure, sawRecovery, sawAdmin, sawResource bool
	failures := 0
	for _, event := range events {
		switch event.Category {
		case "failure":
			failures++
			sawFailure = true
			if cm.Trigger == "" {
				cm.Trigger = event.Summary
			}
		case "recovery":
			sawRecovery = true
			cm.CorrectedBelief = event.Summary
		case "admin_feedback":
			sawAdmin = true
		case "resource_change":
			sawResource = true
			cm.Domain = DomainResources
		case "relationship_shift":
			cm.Domain = DomainRelationships
		case "strategy_shift":
			cm.Domain = DomainRules
		}
	}

	if sawFailure && failures >= 2 {
		cm.DecisionImpact += 0.2
		cm.ReasonCodes = append(cm.ReasonCodes, "repeated_failure")
	}
	if sawRecovery {
		cm.DecisionImpact += 0.2
		cm.ReasonCodes = append(cm.ReasonCodes, "recovery_after_shift")
	}
	if sawAdmin {
		cm.DecisionImpact += 0.1
		cm.ReasonCodes = append(cm.ReasonCodes, "admin_feedback")
	}
	if sawResource {
		cm.DecisionImpact += 0.1
		cm.ReasonCodes = append(cm.ReasonCodes, "resource_signal")
	}

	switch {
	case sawFailure && sawRecovery && sawResource:
		cm.Trigger = "repeated failure survived approved resource change"
		cm.MistakenBelief = "approved capacity or support was the real fix"
		cm.CorrectedBelief = "the real fix came from narrowing the path, not from the approved resource"
		cm.ActionBoundary = "stop spending on the apparent edge when the same failure survives it"
		cm.PreservedCost = "wasted budget, repeated failure, and reputation damage from visible useless retries"
		cm.ScopeLimit = "only applies when the same failure cause survives the spend unchanged"
	case sawFailure && sawRecovery:
		cm.Trigger = "repeated failure only cleared after narrowing the path"
		cm.MistakenBelief = "another broad retry would eventually work"
		cm.CorrectedBelief = "the path itself was too broad and had to be narrowed"
		cm.ActionBoundary = "stop broad retries after same-cause repetition"
		cm.PreservedCost = "wasted retries and blurred diagnosis"
		cm.ScopeLimit = "only applies when the same failure cause repeats"
	case sawAdmin:
		cm.Trigger = "admin feedback exposed a legibility or structure weakness"
		cm.MistakenBelief = "the outcome alone was enough; explanation quality did not matter"
		cm.CorrectedBelief = "legibility changes future trust and future collaboration quality"
		cm.ActionBoundary = "separate fix, cause, and later feedback in the record"
		cm.PreservedCost = "miscoordination and trust erosion"
		cm.ScopeLimit = "only applies when later actors depend on the record"
	default:
		cm.Trigger = "recent event cluster"
		cm.MistakenBelief = "the event was too small to retain"
		cm.CorrectedBelief = "the event still changed future action"
		cm.ActionBoundary = "keep only if the next move changes"
		cm.PreservedCost = "future confusion"
		cm.ScopeLimit = "do not generalize beyond the same event class"
	}

	if cm.DecisionImpact > 1 {
		cm.DecisionImpact = 1
	}
	return cm
}
