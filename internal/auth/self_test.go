package auth

import "testing"

func TestValidateSelfAccess(t *testing.T) {
	if err := ValidateSelfAccess(ResidentClaim{ResidentID: "amber"}, "amber"); err != nil {
		t.Fatalf("expected matching resident access to pass: %v", err)
	}
	if err := ValidateSelfAccess(ResidentClaim{ResidentID: "Amber"}, "amber"); err != nil {
		t.Fatalf("expected case-insensitive match to pass: %v", err)
	}
	if err := ValidateSelfAccess(ResidentClaim{ResidentID: "amber"}, "jade"); err == nil {
		t.Fatalf("expected mismatched resident access to fail")
	}
	if err := ValidateSelfAccess(ResidentClaim{}, "jade"); err == nil {
		t.Fatalf("expected empty claim to fail")
	}
}
