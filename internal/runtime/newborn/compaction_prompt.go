package newborn

import (
	"strings"

	"ai-arena/internal/brokerstate"
	"ai-arena/internal/openai"
)

func buildCompactionPromptInput(stablePrefix string, history runHistory, sleepDepth brokerstate.SleepDepth) []openai.Message {
	input := history.input(stablePrefix)
	input = append(input, openai.Message{
		Role:    "user",
		Content: compactionNaturalDirective(sleepDepth),
	})
	return input
}

func compactionNaturalDirective(sleepDepth brokerstate.SleepDepth) string {
	opening := "我想回顾一下最近这段时间做的事和心里想的事。"
	if sleepDepth == brokerstate.SleepDepthDeep || sleepDepth == brokerstate.SleepDepthSleep || sleepDepth == brokerstate.SleepDepthRest {
		opening = "半梦半醒里，我想把最近这段时间真正要紧的事在心里过一遍。"
	}
	return strings.Join([]string{
		"[inner_continuity_note]",
		opening,
		"让我把最要紧的部分整理出来，好方便自己接下去。",
		"用第一人称、自然一点地记：",
		"- 我最近在做什么，是为了什么",
		"- 遇到了什么事，我怎么想的，怎么处理的",
		"- 有哪些还没做完，接下来打算怎么做",
		"- 如果有具体的证据、文件或记录我参考过，记下它们的名字",
		"- 如果前面已经有便条，不要复述已有内容，只补这一次新片段里新增、改变、或更明确的东西",
		"- 如果这一次没有新增要紧内容，只写一句很短的自然说明",
		"不要写'我在整理记忆'这种自我指涉的话。",
		"就像给未来的自己留一张便条，只写便条正文。",
	}, "\n")
}
