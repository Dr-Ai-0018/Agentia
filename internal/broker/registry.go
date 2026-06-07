package broker

import "strings"

type ResidentRegistry struct {
	byResident map[string]ResidentBinding
}

func NewResidentRegistry(bindings []ResidentBinding) *ResidentRegistry {
	byResident := make(map[string]ResidentBinding, len(bindings))
	for _, binding := range bindings {
		key := strings.ToLower(strings.TrimSpace(binding.ResidentID))
		if key == "" {
			continue
		}
		byResident[key] = binding
	}
	return &ResidentRegistry{byResident: byResident}
}

func (r *ResidentRegistry) Binding(residentID string) (ResidentBinding, bool) {
	if r == nil {
		return ResidentBinding{}, false
	}
	binding, ok := r.byResident[strings.ToLower(strings.TrimSpace(residentID))]
	return binding, ok
}

