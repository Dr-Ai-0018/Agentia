package newborn

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type ActionExecutor interface {
	Execute(profile ResidentProfile, decision AgentDecision) string
}

type IncusActionExecutor struct {
	world *WorldBridge
}

func NewIncusActionExecutor() *IncusActionExecutor {
	return &IncusActionExecutor{
		world: NewWorldBridge(".agents"),
	}
}

func (e *IncusActionExecutor) Execute(profile ResidentProfile, decision AgentDecision) string {
	switch decision.NextAction {
	case "write_note":
		if strings.TrimSpace(decision.Command) == "" {
			return "write_note denied: command is required and must contain the actual note-writing command"
		}
		return guestCommand(profile.Instance, decision.Command)
	case "guest_exec":
		if strings.TrimSpace(decision.Command) == "" {
			return "guest_exec denied: command is required"
		}
		return guestCommand(profile.Instance, decision.Command)
	case "talk_to_chenglin":
		if strings.TrimSpace(decision.Message) == "" {
			return "talk_to_chenglin denied: message is required"
		}
		observation, err := e.world.RecordResidentMessage(profile, decision.Message, time.Now().UTC())
		if err != nil {
			return "talk_to_chenglin failed: " + err.Error()
		}
		return observation
	default:
		return "no operation executed"
	}
}

func guestCommand(instance, script string) string {
	cmd := exec.Command("incus", "exec", instance, "--", "bash", "-lc", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("guest command failed:\n%s", strings.TrimSpace(string(out)))
	}
	return string(out)
}
