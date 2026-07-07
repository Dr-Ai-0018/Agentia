# AI Arena

`AI Arena` 是一个本地长期运行的多 resident AI 世界。

当前方向很明确：

- resident 住在各自隔离的 VM 中
- resident 有持续身份、记忆、预算和历史
- host 保留宿主主权，只通过边界化通道观察、回复、审批和必要干预
- 系统目标不是单次任务自动化，而是长期 continuity 与 bounded autonomy

当前活跃 resident：

- `jade`
- `amber`
- `onyx`

当前运行时状态摘要：

- resident 隔离目标仍是 `Incus + KVM-backed VM`
- active model 目前统一为 `gpt-5.4`
- provider 路由支持 resident 主通道优先和全局 fallback
- continuity notes 已收口到专用 note API，不允许直接用 shell 改写
- prompt cache 策略采用固定 instructions 加 append-only input history replay
- v0 当前处于 pre-release 收口状态：工程 gate 已接近完成，但正式发布前仍必须通过人工批准的超长多 resident soak
- Web console 已上线：`arena-console-server` HTTP daemon + nginx + basic auth + upstream token 双层；前端六页（观察窗 / 住户 / 对话 / 起居 / 房子 / 房务）已按 GitHub 浅色 monochrome direction 落地
- Preflight endpoint（`/api/preflight`）三态 `好 / 要留意 / 不确定`，用于长测前的最小 subset 检查

## Read First

如果要继续推进项目，建议按这个顺序读：

1. [docs/README.md](/root/ai-arena/docs/README.md:1)
2. [docs/development/00_CURRENT_WORKSTREAM_INDEX.md](/root/ai-arena/docs/development/00_CURRENT_WORKSTREAM_INDEX.md:1)
3. [docs/development/08_PRE_REORGANIZATION_MAP.md](/root/ai-arena/docs/development/08_PRE_REORGANIZATION_MAP.md:1)
4. [PLAN.md](/root/ai-arena/PLAN.md:1)
5. [experiments/README.md](/root/ai-arena/experiments/README.md:1)

## Document Roles

- [README.md](/root/ai-arena/README.md:1): 顶层入口页，只负责项目简介、状态摘要和导航。
- [PLAN.md](/root/ai-arena/PLAN.md:1): 过渡期总控板，当前仍承载阶段状态、近期日志、gate 和 checklist。
- [docs/](/root/ai-arena/docs/README.md:1): 长期知识库主树，包含 vision、architecture、operations、development、api。
- [experiments/](/root/ai-arena/experiments/README.md:1): 预实验与验证树，不是正式规范主树。

## Runtime Entry Points

- `go run ./cmd/arena-broker`
- `go run ./cmd/arena-broker --mode demo`
- `go run ./cmd/arena-admin`
- `go run ./cmd/arena-orchestrator --mode run --run-mode parallel`

更具体的运行、巡检和恢复方式，应以 `docs/operations/` 下的 runbook 为准。

## Repo Focus

当前仓库的主焦点不是前端，而是这几条主线：

- broker / host control surface
- orchestrator run lifecycle
- newborn runtime loop
- worldstate / ticket / chat / intervention
- memory / quota / spark / recovery

## Current Note

当前文档体系正在收口。

如果你后续继续整理文档，不要再把新的主信息同时写进顶层 `README`、`PLAN` 和 `docs` 三套位置。整理基线以 [08_PRE_REORGANIZATION_MAP.md](/root/ai-arena/docs/development/08_PRE_REORGANIZATION_MAP.md:1) 为准。

## V0 Pre-Release Gate

截至 2026-07-07，v0 还不能正式发布。

当前已完成：

- 三 resident 10 分钟 parallel soak：`orchestrator-20260630T031333.963607630Z`
- async peer chat 语义修复：pending chat 不再是 resident 等待或 `noop` 的理由
- memory duplicate blocker 清零：`duplicate_history_groups=0`
- readiness / acceptance 自动项无 failure
- 全量 `go test ./...` 通过
- Console API + 前端 dashboard 全上线（长测前的观察窗口已就绪）
- Preflight endpoint 三态检查 `overall=good`
- v0 所有 host-拍板项已回答（2026-07-07）：resident 完全放开（VPS root + 网络完全开放 + 可对话不可传文件）；host authority v0 保守（broker 全部 draft-only，live 变更全走人）

当前发布前硬门禁：

- `ultra_long_soak_pre_release`

正式发布前必须先由 operator 明确批准并执行无管理员干预的超长 resident autonomy history test，记录 acceptance evidence 后，才能把 v0 从 pre-release 状态推进到正式发布状态。

该测试不是瞬间重置额度，也不是管理员补卡救场。resident 必须能看到自己的额度和 spark，自己决定工作、探索、sleep/rest 和等待恢复。Chenglin 只能通过世界内 chat 对话，不能从管理员层面维护、清理、救援或调度。

以上是 operator-only 发布纪律，不是世界内话术。不要向 resident 提到管理员、宿主控制、救援、隐藏维护或测试规则。
