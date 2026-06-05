package sparkledger

import (
	"fmt"
	"math"
	"sort"
	"time"
)

const precision = 10_000

type EntryKind string

const (
	EntryCharge EntryKind = "charge"
	EntryReward EntryKind = "reward"
	EntrySalary EntryKind = "salary"
	EntryBonus  EntryKind = "bonus"
	EntryGrant  EntryKind = "grant"
)

type Entry struct {
	ID              string    `json:"id"`
	ResidentID      string    `json:"resident_id"`
	Kind            EntryKind `json:"kind"`
	SparkDelta      float64   `json:"spark_delta"`
	SparkDeltaUnits int64     `json:"spark_delta_units"`
	BalanceAfter    float64   `json:"balance_after"`
	BalanceUnits    int64     `json:"balance_units"`
	Reason          string    `json:"reason"`
	CreatedAt       time.Time `json:"created_at"`
}

type Account struct {
	ResidentID   string    `json:"resident_id"`
	Balance      float64   `json:"balance"`
	BalanceUnits int64     `json:"balance_units"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Ledger struct {
	account Account
	entries []Entry
	nextSeq int
}

func New(residentID string) *Ledger {
	return &Ledger{
		account: Account{
			ResidentID: residentID,
		},
		entries: make([]Entry, 0, 16),
		nextSeq: 1,
	}
}

func Restore(account Account, entries []Entry) *Ledger {
	copied := make([]Entry, len(entries))
	copy(copied, entries)
	nextSeq := len(copied) + 1
	return &Ledger{
		account: account,
		entries: copied,
		nextSeq: nextSeq,
	}
}

func (l *Ledger) Account() Account {
	return l.account
}

func (l *Ledger) Entries() []Entry {
	out := make([]Entry, len(l.entries))
	copy(out, l.entries)
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (l *Ledger) Credit(kind EntryKind, spark float64, reason string, at time.Time) (Entry, error) {
	if spark <= 0 {
		return Entry{}, fmt.Errorf("credit spark must be positive")
	}
	return l.append(kind, spark, reason, at)
}

func (l *Ledger) Debit(kind EntryKind, spark float64, reason string, at time.Time) (Entry, error) {
	if spark <= 0 {
		return Entry{}, fmt.Errorf("debit spark must be positive")
	}
	if l.account.BalanceUnits < toUnits(spark) {
		return Entry{}, fmt.Errorf("insufficient spark balance")
	}
	return l.append(kind, -spark, reason, at)
}

func (l *Ledger) DebitAllowDebt(kind EntryKind, spark float64, reason string, at time.Time) (Entry, error) {
	if spark <= 0 {
		return Entry{}, fmt.Errorf("debit spark must be positive")
	}
	return l.appendWithPolicy(kind, -spark, reason, at, true)
}

func (l *Ledger) append(kind EntryKind, spark float64, reason string, at time.Time) (Entry, error) {
	return l.appendWithPolicy(kind, spark, reason, at, false)
}

func (l *Ledger) appendWithPolicy(kind EntryKind, spark float64, reason string, at time.Time, allowDebt bool) (Entry, error) {
	deltaUnits := toUnits(spark)
	nextUnits := l.account.BalanceUnits + deltaUnits
	if nextUnits < 0 && !allowDebt {
		return Entry{}, fmt.Errorf("spark balance cannot go negative")
	}

	l.account.BalanceUnits = nextUnits
	l.account.Balance = fromUnits(nextUnits)
	l.account.UpdatedAt = at

	entry := Entry{
		ID:              fmt.Sprintf("spark-%04d", l.nextSeq),
		ResidentID:      l.account.ResidentID,
		Kind:            kind,
		SparkDelta:      fromUnits(deltaUnits),
		SparkDeltaUnits: deltaUnits,
		BalanceAfter:    l.account.Balance,
		BalanceUnits:    nextUnits,
		Reason:          reason,
		CreatedAt:       at,
	}
	l.nextSeq++
	l.entries = append(l.entries, entry)
	return entry, nil
}

func toUnits(v float64) int64 {
	return int64(math.Round(v * precision))
}

func fromUnits(v int64) float64 {
	return float64(v) / precision
}
