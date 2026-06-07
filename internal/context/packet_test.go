package context

import (
	"strings"
	"testing"
)

func TestPromptCacheKeyStableAcrossWorkingShift(t *testing.T) {
	base := Build(BuildSpec{
		Identity: ResidentIdentity{
			Name:     "amber",
			Model:    "gpt-5.5",
			Persona:  "coordinator",
			Style:    "clear",
			CoreBias: "reduce confusion",
		},
		WorldState: "same-world",
		MemoryDigest: MemoryDigest{
			Identity: "same-identity",
			Resource: "same-resource",
			Strategy: "same-strategy",
		},
		Working: WorkingContext{
			RemainingSeconds: 300,
			UsedActions:      map[string]int{"guest_exec": 1},
		},
	})
	shifted := Build(BuildSpec{
		Identity: ResidentIdentity{
			Name:     "amber",
			Model:    "gpt-5.5",
			Persona:  "coordinator",
			Style:    "clear",
			CoreBias: "reduce confusion",
		},
		WorldState: "same-world",
		MemoryDigest: MemoryDigest{
			Identity: "same-identity",
			Resource: "same-resource",
			Strategy: "same-strategy",
		},
		Working: WorkingContext{
			RemainingSeconds: 120,
			UsedActions:      map[string]int{"guest_exec": 2, "talk_to_chenglin": 1},
			LastObservation:  "working state changed",
		},
	})
	if base.PromptCacheKey("amber") != shifted.PromptCacheKey("amber") {
		t.Fatalf("working-shift should not change prompt cache key")
	}
	if base.FullInput() == shifted.FullInput() {
		t.Fatalf("working-shift should change full input")
	}
}

func TestPromptCacheKeyChangesOnStablePrefixShift(t *testing.T) {
	base := Build(BuildSpec{
		Identity:   ResidentIdentity{Name: "jade", Model: "gpt-5.4", Persona: "steady", Style: "plain", CoreBias: "stability"},
		WorldState: "world-a",
		MemoryDigest: MemoryDigest{
			Identity: "identity-a",
		},
	})
	shifted := Build(BuildSpec{
		Identity:   ResidentIdentity{Name: "jade", Model: "gpt-5.4", Persona: "steady", Style: "plain", CoreBias: "stability"},
		WorldState: "world-b",
		MemoryDigest: MemoryDigest{
			Identity: "identity-a",
		},
	})
	if base.PromptCacheKey("jade") == shifted.PromptCacheKey("jade") {
		t.Fatalf("world shift should change prompt cache key")
	}
}

func TestMemoryGovernanceSectionRenders(t *testing.T) {
	packet := Build(BuildSpec{
		Identity: ResidentIdentity{
			Name:     "amber",
			Model:    "gpt-5.5",
			Persona:  "coordinator",
			Style:    "clear",
			CoreBias: "reduce confusion",
		},
		WorldState: "same-world",
		MemoryDigest: MemoryDigest{
			Identity:   "same-identity",
			Resource:   "same-resource",
			Strategy:   "same-strategy",
			Governance: []string{"memory=amber-short-raw layer=short quality=low review=needs_resident_review reason=This looks too close to a raw log excerpt. resident_options=keep|rewrite|compress|demote|delete"},
		},
		Working: WorkingContext{
			RemainingSeconds: 90,
		},
	})
	if !strings.Contains(packet.FullInput(), "memory_governance:") {
		t.Fatalf("expected memory governance section in packet")
	}
	if !strings.Contains(packet.FullInput(), "resident_options=keep|rewrite|compress|demote|delete") {
		t.Fatalf("expected resident governance options in packet")
	}
}

func TestMemoryReviewQueueRendersInWorkingContext(t *testing.T) {
	packet := Build(BuildSpec{
		Identity: ResidentIdentity{
			Name:     "amber",
			Model:    "gpt-5.5",
			Persona:  "coordinator",
			Style:    "clear",
			CoreBias: "reduce confusion",
		},
		WorldState: "same-world",
		MemoryDigest: MemoryDigest{
			Identity: "same-identity",
		},
		Working: WorkingContext{
			RemainingSeconds: 90,
			MemoryReview: []string{
				"memory=amber-short-raw layer=short quality=low review=needs_resident_review reason=This looks too close to a raw log excerpt. resident_options=keep|rewrite|compress|demote|delete",
			},
		},
	})
	if !strings.Contains(packet.FullInput(), "memory_review_queue:") {
		t.Fatalf("expected memory review queue in working context")
	}
}

func TestFreshWorldUpdatesRenderInWorkingContext(t *testing.T) {
	packet := Build(BuildSpec{
		Identity: ResidentIdentity{
			Name:     "amber",
			Model:    "gpt-5.5",
			Persona:  "coordinator",
			Style:    "clear",
			CoreBias: "reduce confusion",
		},
		WorldState: "same-world",
		MemoryDigest: MemoryDigest{
			Identity: "same-identity",
		},
		Working: WorkingContext{
			RemainingSeconds: 90,
			FreshWorldUpdates: []string{
				"[2026-06-07T07:00:01Z] Chenglin said hello back.",
			},
		},
	})
	if !strings.Contains(packet.FullInput(), "fresh_world_updates:") {
		t.Fatalf("expected fresh world updates in working context")
	}
	if !strings.Contains(packet.FullInput(), "Chenglin said hello back.") {
		t.Fatalf("expected fresh world update body")
	}
}
