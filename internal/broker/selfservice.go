package broker

import (
	"fmt"

	"ai-arena/internal/auth"
	"ai-arena/internal/brokerstate"
)

type SelfService struct {
	app *App
}

func NewSelfService(app *App) *SelfService {
	return &SelfService{app: app}
}

func (s *SelfService) Status(claim auth.ResidentClaim) (brokerstate.ResidentStatus, error) {
	if err := auth.ValidateSelfAccess(claim, claim.ResidentID); err != nil {
		return brokerstate.ResidentStatus{}, err
	}
	return s.app.RunStatus(claim.ResidentID)
}

func (s *SelfService) Binding(claim auth.ResidentClaim) (ResidentBinding, error) {
	if err := auth.ValidateSelfAccess(claim, claim.ResidentID); err != nil {
		return ResidentBinding{}, err
	}
	binding, ok := s.app.Binding(claim.ResidentID)
	if !ok {
		return ResidentBinding{}, fmt.Errorf("unknown resident binding: %s", claim.ResidentID)
	}
	return binding, nil
}
