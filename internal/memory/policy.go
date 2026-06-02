package memory

import "time"

type Layer string

const (
	LayerInstant   Layer = "instant"
	LayerShort     Layer = "short"
	LayerLong      Layer = "long"
	LayerPermanent Layer = "permanent"
)

type Domain string

const (
	DomainIdentity      Domain = "identity"
	DomainRules         Domain = "rules"
	DomainResources     Domain = "resources"
	DomainRelationships Domain = "relationships"
	DomainLessons       Domain = "lessons"
	DomainReflections   Domain = "reflections"
	DomainHistory       Domain = "history"
	DomainWorking       Domain = "working"
)

type Action string

const (
	ActionCreate    Action = "create"
	ActionUpdate    Action = "update"
	ActionPromote   Action = "promote"
	ActionRetain    Action = "retain"
	ActionDecay     Action = "decay"
	ActionDelete    Action = "delete"
	ActionReview    Action = "review"
	ActionSummarize Action = "summarize"
)

type EventSignal struct {
	Domain                Domain
	Novelty               float64
	DecisionImpact        float64
	ImpactRounds          int
	Recurrence            int
	IdentityWeight        float64
	ResourceWeight        float64
	RelationshipWeight    float64
	RuleWeight            float64
	Confidence            float64
	AgeSinceTouch         time.Duration
	AgeSinceCreation      time.Duration
	Contradicted          bool
	UserPinned            bool
	NeedsAdminAudit       bool
	DailyReflectionAnchor bool
}

type Decision struct {
	Action            Action
	TargetLayer       Layer
	TTL               time.Duration
	ReviewAfter       time.Duration
	NeedsConfirmation bool
	ReasonCodes       []string
}

type Policy struct {
	InstantTTL        time.Duration
	ShortTTL          time.Duration
	LongTTL           time.Duration
	PermanentReview   time.Duration
	DailySummaryAfter time.Duration
}

func DefaultPolicy() Policy {
	return Policy{
		InstantTTL:        6 * time.Hour,
		ShortTTL:          72 * time.Hour,
		LongTTL:           21 * 24 * time.Hour,
		PermanentReview:   30 * 24 * time.Hour,
		DailySummaryAfter: 24 * time.Hour,
	}
}

func (p Policy) Evaluate(signal EventSignal) Decision {
	if signal.UserPinned {
		return Decision{
			Action:      ActionRetain,
			TargetLayer: LayerPermanent,
			ReviewAfter: p.PermanentReview,
			ReasonCodes: []string{"user_pinned"},
		}
	}

	if signal.Contradicted {
		return Decision{
			Action:            ActionReview,
			TargetLayer:       LayerShort,
			ReviewAfter:       0,
			NeedsConfirmation: signal.NeedsAdminAudit,
			ReasonCodes:       []string{"contradicted"},
		}
	}

	score := signal.DecisionImpact*0.35 +
		minFloat(float64(signal.ImpactRounds)/10, 1)*0.2 +
		minFloat(float64(signal.Recurrence)/3, 1)*0.15 +
		signal.IdentityWeight*0.1 +
		signal.ResourceWeight*0.08 +
		signal.RelationshipWeight*0.07 +
		signal.RuleWeight*0.05

	if signal.Confidence < 0.45 && score < 0.55 {
		return Decision{
			Action:      ActionCreate,
			TargetLayer: LayerInstant,
			TTL:         p.InstantTTL,
			ReasonCodes: []string{"weak_confidence", "low_decision_stability"},
		}
	}

	if signal.IdentityWeight >= 0.75 || signal.RuleWeight >= 0.85 {
		return Decision{
			Action:      ActionPromote,
			TargetLayer: LayerPermanent,
			ReviewAfter: p.PermanentReview,
			ReasonCodes: []string{"identity_or_rule_anchor"},
		}
	}

	if signal.DecisionImpact >= 0.7 && signal.ImpactRounds >= 3 && signal.Recurrence >= 2 {
		return Decision{
			Action:      ActionPromote,
			TargetLayer: LayerLong,
			TTL:         p.LongTTL,
			ReviewAfter: 7 * 24 * time.Hour,
			ReasonCodes: []string{"multi_round_decision_impact"},
		}
	}

	if signal.DecisionImpact >= 0.45 || signal.ImpactRounds >= 2 || signal.ResourceWeight >= 0.6 || signal.RelationshipWeight >= 0.6 {
		return Decision{
			Action:      ActionCreate,
			TargetLayer: LayerShort,
			TTL:         p.ShortTTL,
			ReviewAfter: 24 * time.Hour,
			ReasonCodes: []string{"near_term_behavior_shift"},
		}
	}

	return Decision{
		Action:      ActionCreate,
		TargetLayer: LayerInstant,
		TTL:         p.InstantTTL,
		ReasonCodes: []string{"low_endurance_signal"},
	}
}

func (p Policy) EvaluateDecay(layer Layer, signal EventSignal) Decision {
	if signal.UserPinned {
		return Decision{
			Action:      ActionRetain,
			TargetLayer: LayerPermanent,
			ReviewAfter: p.PermanentReview,
			ReasonCodes: []string{"user_pinned"},
		}
	}

	switch layer {
	case LayerInstant:
		if signal.AgeSinceTouch >= p.InstantTTL {
			return Decision{Action: ActionDelete, TargetLayer: LayerInstant, ReasonCodes: []string{"instant_expired"}}
		}
	case LayerShort:
		if signal.AgeSinceTouch >= p.ShortTTL {
			return Decision{Action: ActionDecay, TargetLayer: LayerInstant, TTL: p.InstantTTL, ReasonCodes: []string{"short_expired"}}
		}
	case LayerLong:
		if signal.AgeSinceTouch >= p.LongTTL {
			return Decision{
				Action:            ActionReview,
				TargetLayer:       LayerShort,
				ReviewAfter:       0,
				NeedsConfirmation: true,
				ReasonCodes:       []string{"long_needs_revalidation"},
			}
		}
	case LayerPermanent:
		if signal.AgeSinceTouch >= p.PermanentReview || signal.AgeSinceCreation >= p.PermanentReview {
			return Decision{
				Action:            ActionReview,
				TargetLayer:       LayerPermanent,
				ReviewAfter:       p.PermanentReview,
				NeedsConfirmation: true,
				ReasonCodes:       []string{"permanent_review_due"},
			}
		}
	}

	return Decision{Action: ActionRetain, TargetLayer: layer, ReasonCodes: []string{"still_active"}}
}

func (p Policy) DailySummaryDue(lastSummary, now time.Time) bool {
	if lastSummary.IsZero() {
		return true
	}
	return now.Sub(lastSummary) >= p.DailySummaryAfter
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
