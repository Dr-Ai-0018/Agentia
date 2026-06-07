package broker

import (
	"fmt"

	"ai-arena/internal/auth"
	"ai-arena/internal/audit"
	"ai-arena/internal/brokerstate"
	"ai-arena/internal/world"
)

type SelfService struct {
	app     *App
	machine MachineControl
	audit   *audit.Logger
	history *world.History
	guard   *Guard
}

func NewSelfService(app *App) *SelfService {
	return &SelfService{
		app:     app,
		machine: NewIncusMachineControl(),
		audit:   audit.New(app.root),
		history: world.New(app.root),
		guard:   NewGuard(),
	}
}

func (s *SelfService) Status(claim auth.ResidentClaim) (brokerstate.ResidentStatus, error) {
	if err := s.guard.Allow(ActionStatus); err != nil {
		return brokerstate.ResidentStatus{}, err
	}
	if err := auth.ValidateSelfAccess(claim, claim.ResidentID); err != nil {
		return brokerstate.ResidentStatus{}, err
	}
	return s.app.RunStatus(claim.ResidentID)
}

func (s *SelfService) Binding(claim auth.ResidentClaim) (ResidentBinding, error) {
	if err := s.guard.Allow(ActionBinding); err != nil {
		return ResidentBinding{}, err
	}
	if err := auth.ValidateSelfAccess(claim, claim.ResidentID); err != nil {
		return ResidentBinding{}, err
	}
	binding, ok := s.app.Binding(claim.ResidentID)
	if !ok {
		return ResidentBinding{}, fmt.Errorf("unknown resident binding: %s", claim.ResidentID)
	}
	return binding, nil
}
