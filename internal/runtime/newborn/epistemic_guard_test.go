package newborn

import (
	"strings"
	"testing"

	"ai-arena/internal/brokerstate"
	"ai-arena/internal/openai"
)

func TestCompactionNaturalDirectiveAvoidsMetaTerms(t *testing.T) {
	directive := compactionNaturalDirective("")
	for _, banned := range []string{
		"context",
		"token",
		"compaction",
		"summary",
		"runtime",
		"harness",
		"session",
		"上下文",
		"压缩",
		"摘要",
		"会话",
		"管理员",
	} {
		if strings.Contains(strings.ToLower(directive), banned) {
			t.Fatalf("directive leaked meta term %q in %q", banned, directive)
		}
	}
}

func TestCompactionNaturalDirectiveUsesDreamFramingDuringSleep(t *testing.T) {
	got := compactionNaturalDirective(brokerstate.SleepDepthDeep)
	if !strings.Contains(got, "半梦半醒") {
		t.Fatalf("expected dream framing during sleep, got %q", got)
	}
}

func TestBuildCompactionPromptInputPreservesStablePrefix(t *testing.T) {
	history := newRunHistoryForPurpose("", 60)
	history.recent = []openai.Message{{Role: "user", Content: "round text"}}
	input := buildCompactionPromptInput("[stable]", history, "")
	if len(input) < 3 || input[0].Content != "[stable]" {
		t.Fatalf("stable prefix must remain first, got %#v", input)
	}
	if !strings.Contains(input[len(input)-1].Content, "[inner_continuity_note]") {
		t.Fatalf("expected natural directive as final message, got %#v", input)
	}
}

func TestCompactionEpistemicGuardAllowsWorldInternalChenglin(t *testing.T) {
	text := "我和程林聊过那件小事，然后继续看 boot-notes.md。"
	if violation := checkCompactionEpistemicGuard(text); violation != nil {
		t.Fatalf("Chenglin is world-internal and should be allowed, got %#v", violation)
	}
}

func TestCompactionEpistemicGuardRejectsMetaTermsWithRedactedSample(t *testing.T) {
	violation := checkCompactionEpistemicGuard("我注意到 context window 快满了，需要压缩。")
	if violation == nil {
		t.Fatal("expected guard violation")
	}
	if violation.Term == "" {
		t.Fatalf("expected offending term, got %#v", violation)
	}
	if strings.Contains(strings.ToLower(violation.Sample), "context") || strings.Contains(violation.Sample, "压缩") {
		t.Fatalf("guard sample must be redacted, got %q", violation.Sample)
	}

	violation = checkCompactionEpistemicGuard("外部上下文上，我前面已经给程林发过消息。")
	if violation == nil || violation.Term != "上下文" {
		t.Fatalf("expected generic Chinese context term to trip guard, got %#v", violation)
	}
}

func TestCompactionEpistemicGuardUsesASCIIBoundaries(t *testing.T) {
	if violation := checkCompactionEpistemicGuard("I kept the discussion concrete and grounded."); violation != nil {
		t.Fatalf("discussion should not trip session guard: %#v", violation)
	}
	if violation := checkCompactionEpistemicGuard("I noticed the session label in my notes."); violation == nil || violation.Term != "session" {
		t.Fatalf("expected session to trip guard, got %#v", violation)
	}
}
