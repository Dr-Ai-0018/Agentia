package brokerstate

import (
	"fmt"
	"strings"
	"time"

	"ai-arena/internal/recovery"
	"ai-arena/internal/runtimecore"
	"ai-arena/internal/tokenledger"
)

type ResidentProfile struct {
	ResidentID      string
	InitialQuota    tokenledger.QuotaState
	InitialGrant    float64
	RecoveryPolicy  recovery.Policy
}

type Registry struct {
	profiles map[string]ResidentProfile
}

func NewRegistry(profiles []ResidentProfile) *Registry {
	index := make(map[string]ResidentProfile, len(profiles))
	for _, profile := range profiles {
		index[profile.ResidentID] = profile
	}
	return &Registry{profiles: index}
}

func (r *Registry) Profile(residentID string) (ResidentProfile, error) {
	profile, ok := r.profiles[residentID]
	if !ok {
		return ResidentProfile{}, fmt.Errorf("unknown resident: %s", residentID)
	}
	return profile, nil
}

func (r *Registry) LoadOrInitEngine(store *Store, cfg runtimecore.Config, residentID string, now time.Time) (*runtimecore.Engine, bool, string, uint64, error) {
	snapshot, path, err := store.LoadResidentSnapshot(residentID)
	if err == nil {
		return runtimecore.Restore(cfg, snapshot), true, path, snapshot.Revision, nil
	}
	if err != nil && !isMissingSnapshot(err) {
		if _, quarantineErr := store.QuarantineResidentSnapshot(residentID, "corrupt", now); quarantineErr != nil {
			return nil, false, "", 0, fmt.Errorf("load snapshot: %w; quarantine snapshot: %v", err, quarantineErr)
		}
	}

	profile, profileErr := r.Profile(residentID)
	if profileErr != nil {
		return nil, false, "", 0, profileErr
	}

	engine := runtimecore.New(cfg, residentID, profile.InitialQuota, now)
	if profile.InitialGrant > 0 {
		if _, creditErr := engine.SparkLedger().Credit("grant", profile.InitialGrant, "registry bootstrap grant", now); creditErr != nil {
			return nil, false, "", 0, creditErr
		}
	}
	return engine, false, "", 0, nil
}

func isMissingSnapshot(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(strings.ToLower(err.Error()), "no such file or directory") {
		return true
	}
	return false
}
