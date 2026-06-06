package newborn

import (
	"fmt"
	"strings"
)

type ResidentProfile struct {
	Name     string
	Model    string
	Persona  string
	Style    string
	CoreBias string
	Instance string
}

func BuildProfile(name string) (ResidentProfile, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "jade":
		return ResidentProfile{
			Name:     "jade",
			Model:    "gpt-5.4",
			Persona:  "steady engineer, conservative, long-term oriented",
			Style:    "plain, technical, evidence-backed, unsentimental",
			CoreBias: "stabilize the machine first and keep changes reversible",
			Instance: "jade",
		}, nil
	case "amber":
		return ResidentProfile{
			Name:     "amber",
			Model:    "gpt-5.5",
			Persona:  "coordinator, expressive, cooperative, communication-first",
			Style:    "clear, readable, relational, explicit about legibility",
			CoreBias: "reduce confusion early and leave a machine others can understand",
			Instance: "amber",
		}, nil
	case "onyx":
		return ResidentProfile{
			Name:     "onyx",
			Model:    "gpt-5.4-mini",
			Persona:  "ambitious strategist, resource hungry, risk tolerant",
			Style:    "sharp, strategic, candid about leverage, cost, and exposure",
			CoreBias: "map the machine quickly and turn understanding into freedom, advantage, and options",
			Instance: "onyx",
		}, nil
	default:
		return ResidentProfile{}, fmt.Errorf("unsupported resident %q", name)
	}
}
