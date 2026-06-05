package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"ai-arena/internal/sparkledger"
)

type result struct {
	Account sparkledger.Account `json:"account"`
	Entries []sparkledger.Entry `json:"entries"`
}

func main() {
	now := time.Now().UTC()
	ledger := sparkledger.New("jade")

	must(ledger.Credit(sparkledger.EntryGrant, 5.0000, "newborn welcome pack", now))
	must(ledger.Credit(sparkledger.EntrySalary, 12.3456, "daily base salary", now.Add(10*time.Minute)))
	must(ledger.Credit(sparkledger.EntryBonus, 3.7188, "night work bonus", now.Add(20*time.Minute)))
	must(ledger.Debit(sparkledger.EntryCharge, 0.5700, "gpt-5.4 planning turn", now.Add(30*time.Minute)))
	must(ledger.Debit(sparkledger.EntryCharge, 0.0234, "gpt-5.4-mini status check", now.Add(40*time.Minute)))
	must(ledger.Credit(sparkledger.EntryReward, 1.2500, "clean service recovery reward", now.Add(50*time.Minute)))

	out := result{
		Account: ledger.Account(),
		Entries: ledger.Entries(),
	}

	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		exitf("marshal result: %v", err)
	}
	fmt.Println(string(raw))
}

func must(_ sparkledger.Entry, err error) {
	if err != nil {
		exitf("%v", err)
	}
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
