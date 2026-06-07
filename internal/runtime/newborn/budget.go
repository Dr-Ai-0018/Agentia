package newborn

import (
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/brokerstate"
	"ai-arena/internal/openai"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

type BudgetController struct {
	brokerApp *broker.App
}

func NewBudgetController(app *broker.App) *BudgetController {
	return &BudgetController{brokerApp: app}
}

func (b *BudgetController) ResetResident(residentID string, now time.Time) error {
	_, err := b.brokerApp.RunReset(residentID, now)
	return err
}

func (b *BudgetController) Preflight(profile ResidentProfile, state loopState, startedAt time.Time) (*brokerstate.PreparedAdmission, error) {
	spec := preflightSpec(profile, state, startedAt)
	prepared, err := b.brokerApp.RunPrepareSpec(profile.Name, spec)
	if err != nil {
		return nil, err
	}
	return &prepared, nil
}

func (b *BudgetController) Settle(profile ResidentProfile, result openai.StreamResult, startedAt time.Time, kind runtimeguard.CallKind, activity tokenledger.ActivityType) (*BrokerUsageLog, error) {
	spec := broker.SpecFromUsage(
		kind,
		tokenledger.Usage{
			InputTokens:  result.InputTokens,
			CachedTokens: result.CachedTokens,
			OutputTokens: result.OutputTokens,
			TotalTokens:  result.InputTokens + result.OutputTokens,
			Model:        profile.Model,
			ResponseID:   result.ResponseID,
			StartedAt:    startedAt,
			FinishedAt:   startedAt.Add(4 * time.Second),
		},
		tokenledger.Penalties{},
		activity,
	)
	resp, err := b.brokerApp.RunAdmitSpec(profile.Name, spec, true)
	if err != nil {
		return nil, err
	}

	log := &BrokerUsageLog{
		Applied:            resp.Applied,
		Denied:             resp.Denied,
		DeniedReason:       append([]string(nil), resp.DeniedReason...),
		BeforeSpark:        resp.BeforeStatus.SparkBalance,
		BeforeDebtActive:   resp.BeforeStatus.DebtActive,
		PreparedSparkCost:  resp.Prepared.Cost.SparkCost,
		PreparedStrainCost: resp.Prepared.Strain.Rounded,
		Quota:              &resp.Quota,
	}
	if resp.ApplyResult != nil {
		log.SparkDelta = resp.ApplyResult.SparkEntry.SparkDelta
		log.ApplyReason = resp.ApplyResult.SparkEntry.Reason
	}
	if resp.AfterStatus != nil {
		log.AfterSpark = resp.AfterStatus.SparkBalance
		log.AfterDebtActive = resp.AfterStatus.DebtActive
		log.Window6HUsed = resp.AfterStatus.Window6HUsed
		log.DayUsed = resp.AfterStatus.DayUsed
		log.WeekUsed = resp.AfterStatus.WeekUsed
		log.AfterStatus = resp.AfterStatus
	}
	return log, nil
}
