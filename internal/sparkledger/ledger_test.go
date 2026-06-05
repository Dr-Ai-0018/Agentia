package sparkledger

import (
	"testing"
	"time"
)

func TestLedgerCreditAndDebit(t *testing.T) {
	ledger := New("jade")
	now := time.Now().UTC()

	if _, err := ledger.Credit(EntrySalary, 12.3456, "base salary", now); err != nil {
		t.Fatalf("credit: %v", err)
	}
	if _, err := ledger.Debit(EntryCharge, 0.0234, "api usage", now.Add(time.Minute)); err != nil {
		t.Fatalf("debit: %v", err)
	}

	got := ledger.Account().Balance
	want := 12.3222
	if got != want {
		t.Fatalf("balance = %.4f, want %.4f", got, want)
	}
}

func TestLedgerRejectsNegativeBalance(t *testing.T) {
	ledger := New("jade")
	_, err := ledger.Debit(EntryCharge, 1.0, "overspend", time.Now().UTC())
	if err == nil {
		t.Fatalf("expected insufficient balance error")
	}
}

func TestLedgerAllowsDebtWhenExplicitlyRequested(t *testing.T) {
	ledger := New("jade")
	_, err := ledger.DebitAllowDebt(EntryCharge, 1.0, "mandatory final notice", time.Now().UTC())
	if err != nil {
		t.Fatalf("expected debt-allowed debit to succeed: %v", err)
	}
	if ledger.Account().Balance >= 0 {
		t.Fatalf("expected negative balance after debt debit")
	}
}
