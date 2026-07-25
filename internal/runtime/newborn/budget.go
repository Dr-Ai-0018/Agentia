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

func (b *BudgetController) Recover(profile ResidentProfile, state loopState, startedAt time.Time) error {
	mode := recoveryModeForPreflight(state)
	if state.LastDecision != nil && state.LastDecision.NextAction == "sleep" {
		switch {
		case state.LastSleepDepth != "":
			mode = brokerstate.RecoveryModeForSleepDepth(state.LastSleepDepth)
		default:
			sleep, err := b.brokerApp.RunSleepEnd(profile.Name, startedAt)
			if err == nil {
				mode = brokerstate.RecoveryModeForSleepDepth(sleep.Session.Depth)
			}
		}
	}
	_, err := b.brokerApp.RunRecoverToNowWithMode(profile.Name, startedAt, mode)
	return err
}

func (b *BudgetController) PreparePreflight(profile ResidentProfile, state loopState, startedAt time.Time, measuredPromptTokens int) (*brokerstate.PreparedAdmission, error) {
	spec := preflightSpec(profile, state, startedAt, measuredPromptTokens)
	prepared, err := b.brokerApp.RunPrepareSpec(profile.Name, spec)
	if err != nil {
		return nil, err
	}
	return &prepared, nil
}

func (b *BudgetController) Preflight(profile ResidentProfile, state loopState, startedAt time.Time) (*brokerstate.PreparedAdmission, error) {
	if err := b.Recover(profile, state, startedAt); err != nil {
		return nil, err
	}
	return b.PreparePreflight(profile, state, startedAt, 0)
}

func (b *BudgetController) SleepStart(profile ResidentProfile, minutes int, startedAt time.Time, reason string) error {
	_, err := b.brokerApp.RunSleepStart(profile.Name, minutes, startedAt, reason)
	return err
}

func (b *BudgetController) SleepEnd(profile ResidentProfile, endedAt time.Time) (brokerstate.SleepRecordResponse, error) {
	return b.brokerApp.RunSleepEnd(profile.Name, endedAt)
}

func recoveryModeForPreflight(state loopState) string {
	if state.LastDecision != nil && state.LastDecision.NextAction == "sleep" {
		return "rest"
	}
	return "idle"
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
	resp, err := b.brokerApp.RunSettleProviderSpec(profile.Name, spec)
	if err != nil {
		return nil, err
	}

	log := &BrokerUsageLog{
		Applied:              resp.Applied,
		Denied:               resp.Denied,
		DeniedReason:         append([]string(nil), resp.DeniedReason...),
		ProviderCostRecorded: resp.ProviderCostRecorded,
		BeforeSpark:          resp.BeforeStatus.SparkBalance,
		BeforeDebtActive:     resp.BeforeStatus.DebtActive,
		PreparedSparkCost:    resp.Prepared.Cost.SparkCost,
		PreparedStrainCost:   resp.Prepared.Strain.Rounded,
		Quota:                &resp.Quota,
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

func (b *BudgetController) RecordSystemProviderUsage(profile ResidentProfile, result openai.StreamResult, startedAt time.Time, eventKind brokerstate.QuotaEventKind, callKind, costClass string, activity tokenledger.ActivityType, metadata map[string]interface{}) (*BrokerUsageLog, error) {
	finishedAt := startedAt.Add(4 * time.Second)
	if finishedAt.Before(startedAt) {
		finishedAt = startedAt
	}
	out, err := b.brokerApp.RunRecordProviderUsage(broker.ProviderUsageRecord{
		ResidentID: profile.Name,
		Kind:       eventKind,
		CallKind:   callKind,
		Usage: tokenledger.Usage{
			InputTokens:  result.InputTokens,
			CachedTokens: result.CachedTokens,
			OutputTokens: result.OutputTokens,
			TotalTokens:  result.InputTokens + result.OutputTokens,
			Model:        profile.Model,
			ResponseID:   result.ResponseID,
			StartedAt:    startedAt,
			FinishedAt:   finishedAt,
		},
		Penalties: tokenledger.Penalties{},
		Activity:  activity,
		CostClass: costClass,
		Metadata:  metadata,
	})
	if err != nil {
		return nil, err
	}
	return &BrokerUsageLog{
		Applied:              false,
		ProviderCostRecorded: true,
		CostClass:            costClass,
		PreparedSparkCost:    out.Event.SparkCost,
		PreparedStrainCost:   out.Event.StrainCost,
		ApplyReason:          string(eventKind),
	}, nil
}
