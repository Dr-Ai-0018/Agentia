package broker

import "testing"

func TestGuardAllow(t *testing.T) {
	guard := NewGuard()
	if err := guard.Allow(ActionRequestMemory); err != nil {
		t.Fatalf("expected whitelisted action to pass: %v", err)
	}
	if err := guard.Allow(SelfAction("raw_incus_exec")); err == nil {
		t.Fatalf("expected non-whitelisted action to fail")
	}
}
