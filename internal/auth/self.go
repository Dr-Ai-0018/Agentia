package auth

import (
	"fmt"
	"strings"
)

type ResidentClaim struct {
	ResidentID string `json:"resident_id"`
}

func ValidateSelfAccess(claim ResidentClaim, requestedResidentID string) error {
	claimID := strings.ToLower(strings.TrimSpace(claim.ResidentID))
	requestedID := strings.ToLower(strings.TrimSpace(requestedResidentID))

	if claimID == "" {
		return fmt.Errorf("resident claim is required")
	}
	if requestedID == "" {
		return fmt.Errorf("requested resident id is required")
	}
	if claimID != requestedID {
		return fmt.Errorf("self-only access denied: claim=%s requested=%s", claimID, requestedID)
	}
	return nil
}
