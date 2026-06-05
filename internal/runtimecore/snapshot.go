package runtimecore

import (
	"encoding/json"
	"time"

	"ai-arena/internal/sparkledger"
	"ai-arena/internal/tokenledger"
)

type Snapshot struct {
	Version     string              `json:"version"`
	SavedAt     time.Time           `json:"saved_at"`
	State       ResidentState       `json:"state"`
	SparkAccount sparkledger.Account `json:"spark_account"`
	SparkEntries []sparkledger.Entry `json:"spark_entries"`
}

func (e *Engine) Snapshot(now time.Time) Snapshot {
	return Snapshot{
		Version:      "runtimecore/v1",
		SavedAt:      now,
		State:        e.state,
		SparkAccount: e.spark.Account(),
		SparkEntries: e.spark.Entries(),
	}
}

func (s Snapshot) Marshal() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

func Restore(cfg Config, snapshot Snapshot) *Engine {
	engine := &Engine{
		cfg:   cfg,
		state: snapshot.State,
		spark: sparkledger.New(snapshot.State.ResidentID),
	}

	account := snapshot.SparkAccount
	entries := snapshot.SparkEntries
	engine.spark = sparkledger.Restore(account, entries)
	return engine
}

func EmptyQuota() tokenledger.QuotaState {
	return tokenledger.QuotaState{}
}
