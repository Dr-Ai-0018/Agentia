package broker

import "fmt"

type SelfAction string

const (
	ActionStatus         SelfAction = "self_status"
	ActionBinding        SelfAction = "self_binding"
	ActionReboot         SelfAction = "self_reboot"
	ActionSnapshot       SelfAction = "self_snapshot"
	ActionRestore        SelfAction = "self_restore"
	ActionRequestCPU     SelfAction = "self_request_cpu"
	ActionRequestMemory  SelfAction = "self_request_memory"
	ActionRequestDisk    SelfAction = "self_request_disk"
	ActionRequestGPUTime SelfAction = "self_request_gpu_time"
	ActionRequestVPS     SelfAction = "self_request_vps_access"
	ActionSubmitResult   SelfAction = "self_submit_result"
)

type Guard struct{}

func NewGuard() *Guard {
	return &Guard{}
}

func (g *Guard) Allow(action SelfAction) error {
	switch action {
	case ActionStatus,
		ActionBinding,
		ActionReboot,
		ActionSnapshot,
		ActionRestore,
		ActionRequestCPU,
		ActionRequestMemory,
		ActionRequestDisk,
		ActionRequestGPUTime,
		ActionRequestVPS,
		ActionSubmitResult:
		return nil
	default:
		return fmt.Errorf("self-only policy denied action: %s", action)
	}
}
