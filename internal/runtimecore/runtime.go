package runtimecore

import (
	"fmt"
	"time"

	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/sparkledger"
	"ai-arena/internal/tokenledger"
)

type Config struct {
	TokenPolicy    tokenledger.Config
	RecoveryPolicy recovery.Policy
	ReserveSpark   float64
	ReserveStrain  int
}

type ResidentState struct {
	ResidentID      string
	Quota           tokenledger.QuotaState
	Fatigue         int
	SleepDebt       int
	DebtActive      bool
	DebtAmount      float64
	FinalNoticeUsed bool
	RecoveryMode    string
	LastRecoveryAt  time.Time
}

type Engine struct {
	cfg   Config
	state ResidentState
	spark *sparkledger.Ledger
}

type PreparedCall struct {
	Kind     runtimeguard.CallKind
	Usage    tokenledger.Usage
	Cost     tokenledger.CostBreakdown
	Strain   tokenledger.Strain
	Decision runtimeguard.Decision
}

type AppliedCall struct {
	Quota      tokenledger.QuotaUpdate
	Fatigue    tokenledger.FatigueUpdate
	SparkEntry sparkledger.Entry
	State      ResidentState
}

func New(cfg Config, residentID string, quota tokenledger.QuotaState, startedAt time.Time) *Engine {
	return &Engine{
		cfg: cfg,
		state: ResidentState{
			ResidentID:     residentID,
			Quota:          quota,
			RecoveryMode:   "idle",
			LastRecoveryAt: startedAt,
		},
		spark: sparkledger.New(residentID),
	}
}

func (e *Engine) SparkLedger() *sparkledger.Ledger {
	return e.spark
}

func (e *Engine) State() ResidentState {
	state := e.state
	state.RecoveryMode = normalizeRecoveryMode(state.RecoveryMode)
	return state
}

func (e *Engine) PrepareCall(kind runtimeguard.CallKind, usage tokenledger.Usage, penalties tokenledger.Penalties) (PreparedCall, error) {
	cost, err := tokenledger.ComputeCost(e.cfg.TokenPolicy, usage)
	if err != nil {
		return PreparedCall{}, err
	}
	strain := tokenledger.ComputeStrain(e.cfg.TokenPolicy, usage, penalties)
	decision := runtimeguard.Evaluate(runtimeguard.State{
		SparkBalance:    e.spark.Account().Balance,
		Quota:           e.state.Quota,
		Fatigue:         e.state.Fatigue,
		SleepDebt:       e.state.SleepDebt,
		ReserveSpark:    e.cfg.ReserveSpark,
		ReserveStrain:   e.cfg.ReserveStrain,
		DebtActive:      e.state.DebtActive,
		DebtAmount:      e.state.DebtAmount,
		FinalNoticeUsed: e.state.FinalNoticeUsed,
	}, runtimeguard.Request{
		Kind:       kind,
		SparkCost:  cost.SparkCost,
		StrainCost: strain.Rounded,
	})

	return PreparedCall{
		Kind:     kind,
		Usage:    usage,
		Cost:     cost,
		Strain:   strain,
		Decision: decision,
	}, nil
}

func (e *Engine) ApplyCall(prepared PreparedCall, activity tokenledger.ActivityType) (AppliedCall, error) {
	if !prepared.Decision.Allowed {
		return AppliedCall{}, fmt.Errorf("call not allowed")
	}

	quotaUpdate := tokenledger.ApplyQuota(e.state.Quota, prepared.Strain.Rounded)
	fatigue, err := tokenledger.ComputeFatigue(e.cfg.TokenPolicy, activity, prepared.Strain)
	if err != nil {
		return AppliedCall{}, err
	}

	var entry sparkledger.Entry
	if prepared.Kind == runtimeguard.CallKindFinalNotice {
		entry, err = e.spark.DebitAllowDebt(
			sparkledger.EntryCharge,
			prepared.Cost.SparkCost,
			fmt.Sprintf("final notice via %s", prepared.Usage.Model),
			prepared.Usage.FinishedAt,
		)
		e.state.FinalNoticeUsed = true
	} else {
		entry, err = e.spark.Debit(
			sparkledger.EntryCharge,
			prepared.Cost.SparkCost,
			fmt.Sprintf("work call via %s", prepared.Usage.Model),
			prepared.Usage.FinishedAt,
		)
	}
	if err != nil {
		return AppliedCall{}, err
	}

	e.state.Quota = quotaUpdate.After
	e.state.Fatigue += fatigue.FatigueGain
	e.state.SleepDebt += sleepDebtGain(activity, fatigue.FatigueGain)
	if e.spark.Account().Balance < 0 {
		e.state.DebtActive = true
		e.state.DebtAmount = -e.spark.Account().Balance
	} else {
		e.state.DebtActive = false
		e.state.DebtAmount = 0
	}

	return AppliedCall{
		Quota:      quotaUpdate,
		Fatigue:    fatigue,
		SparkEntry: entry,
		State:      e.state,
	}, nil
}

func (e *Engine) TickRecovery(now time.Time) recovery.TickResult {
	tick := recovery.Apply(e.cfg.RecoveryPolicy, recovery.State{
		SparkBalance: e.spark.Account().Balance,
		Quota:        e.state.Quota,
		Fatigue:      e.state.Fatigue,
		SleepDebt:    e.state.SleepDebt,
		DebtActive:   e.state.DebtActive,
		DebtAmount:   e.state.DebtAmount,
		RecoveryMode: e.state.RecoveryMode,
		LastTickAt:   e.state.LastRecoveryAt,
	}, now)

	e.state.Quota = tick.QuotaAfter
	e.state.Fatigue = tick.FatigueAfter
	e.state.SleepDebt = tick.SleepDebtAfter
	e.state.DebtActive = tick.DebtActive
	e.state.DebtAmount = tick.DebtAmount
	e.state.RecoveryMode = tick.RecoveryMode
	e.state.LastRecoveryAt = now

	currentUnits := e.spark.Account().BalanceUnits
	targetUnits := int64(tick.NewSparkBalance * 10_000)
	deltaUnits := targetUnits - currentUnits
	if deltaUnits > 0 {
		_, _ = e.spark.Credit(sparkledger.EntryGrant, float64(deltaUnits)/10_000, "recovery tick", now)
	} else if deltaUnits < 0 {
		_, _ = e.spark.DebitAllowDebt(sparkledger.EntryCharge, float64(-deltaUnits)/10_000, "recovery debt settlement", now)
	}

	return tick
}

func (e *Engine) SetRecoveryMode(mode string) {
	e.state.RecoveryMode = normalizeRecoveryMode(mode)
}

func normalizeRecoveryMode(mode string) string {
	switch mode {
	case "rest", "idle", "normal", "deep":
		return mode
	default:
		return "idle"
	}
}

func sleepDebtGain(activity tokenledger.ActivityType, fatigueGain int) int {
	if fatigueGain <= 0 {
		return 0
	}
	switch activity {
	case tokenledger.ActivityStatusCheck:
		return 0
	case tokenledger.ActivityLightWork:
		return maxInt(1, fatigueGain/600)
	case tokenledger.ActivityDeepWork:
		return maxInt(1, fatigueGain/300)
	default:
		return maxInt(1, fatigueGain/400)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
