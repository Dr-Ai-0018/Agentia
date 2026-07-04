package consoleapi

import "testing"

func TestValidateWorldReplyRequiresBoundaryAck(t *testing.T) {
	if err := validateWorldReply("你可以按自己的节奏继续。", false); err == nil {
		t.Fatalf("expected boundary ack error")
	}
}

func TestValidateWorldReplyRejectsPrivateTerms(t *testing.T) {
	for _, body := range []string{
		"dashboard 显示你需要休息",
		"这个 run id 看起来没问题",
		"后台测试规则是这样的",
		"orchestrator-20260630T031333Z 这轮没有问题",
		"你的 phase 和 round 看起来正常",
		"预算和审计显示你需要维护",
		"spark 余额还有 381",
		"这不是 P0，只是 billing spend 问题",
	} {
		if err := validateWorldReply(body, true); err == nil {
			t.Fatalf("expected private boundary term rejection for %q", body)
		}
	}
}

func TestValidateWorldReplyAllowsWorldSafeBody(t *testing.T) {
	body := "你可以继续做自己觉得有价值的事情。额度紧的时候先看 self_quota，也可以按自己的节奏休息一会儿。"
	if err := validateWorldReply(body, true); err != nil {
		t.Fatalf("expected world-safe body to pass: %v", err)
	}
}
