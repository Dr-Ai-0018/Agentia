# AI Arena Plan

## Status Of This File

本文件当前仍是过渡期总控板，不是最终形态的单一 authoritative 文档。

它现在暂时混合承载：

- 项目目标与方向
- 当前 runtime 状态
- 近期 development log
- 阶段计划
- gate / checklist / next work

整理期间统一按下面口径使用本文件：

- 可以继续保留并更新
- 但新增内容应优先写入 `docs/` 对应位置，再视情况回写摘要
- 不再把本文件扩写成新的总知识库

相关整理基线见：

- `docs/development/08_PRE_REORGANIZATION_MAP.md`
- `docs/README.md`
- `docs/development/00_CURRENT_WORKSTREAM_INDEX.md`

## Planned Migration Of This File

后续应逐步把本文件中的内容迁往：

- 稳定结构与边界 -> `docs/architecture/`
- 运行与维护流程 -> `docs/operations/`
- 当前阶段计划、gate、checklist -> `docs/development/`
- 详细阶段日志与历史记录 -> `docs/development/journal/` 或等价位置

在迁移完成前，本文件继续保留“总控板”职责，但不应再成为唯一入口。

## Objective

Build a local long-running multi-resident environment where each AI lives inside its own isolated VM, manages its own limited resources, forms its own memory and strategy, and interacts with the host through bounded channels without ever receiving host-level control.

## Chosen Direction

Use:

- `Incus`
- KVM-backed `VM` instances

Do not use resident-facing LXC containers as the primary isolation model.

## Why This Direction

This system is not a normal trusted automation setup. Residents may:

- self-modify
- run arbitrary code
- attempt persistence
- consume excessive resources
- interfere with boundaries if they are weak

That makes VM isolation the correct default.

## Host Facts

Measured on this machine:

- `6` vCPU
- `23 GiB` RAM
- about `21 GiB` RAM available after stopping Overleaf
- low current CPU load
- `/dev/kvm` exists
- CPU virtualization flags are present

CPU benchmark notes from `/root/test_result.txt`:

- single-core sysbench: `3864`
- 6-thread sysbench: `23139`

Interpretation:

- per-core performance is good
- total concurrency is limited by 6 vCPU
- RAM is not the first bottleneck

## Initial Capacity Decision

Recommended starting layout:

### Option A1: Conservative

- 3 VMs
- each VM: `1 vCPU`, `2 GiB RAM`, `16 GiB disk`

Why:

- leaves comfortable room for host services
- reduces scheduler contention
- easier to observe and tune

### Option A2: Slightly More Aggressive

- 4 VMs
- each VM: `1 vCPU`, `2 GiB RAM`, `12-16 GiB disk`

Why:

- still feasible on this host
- should work if residents are not all doing heavy compute simultaneously

## Current Runtime Status

As of 2026-06-30:

- Three resident VMs are the active target: `jade`, `amber`, and `onyx`.
- Resident isolation remains KVM-backed VM first. LXC is not the resident isolation target.
- Starting resource envelope is `1 vCPU`, `2 GiB RAM`, and `12 GiB disk` per resident VM.
- The runtime uses split action tools instead of one large mixed schema.
- Continuity files under `/root/arena-notes` are a dedicated note-tool surface, not a shell-write surface.
- Resident model distribution is currently `jade=gpt-5.4`, `amber=gpt-5.4`, `onyx=gpt-5.4`.
- Provider routing supports per-resident main channels before global fallback channels.
- Streaming is required for live provider checks and runtime calls.
- Budget reporting can show run spend, resident balances, and cache hit ratios.
- Latest clean three-resident parallel soak: `orchestrator-20260630T031333.963607630Z`.
- That run finished all three residents on `duration_elapsed`: `jade=54`, `amber=58`, `onyx=64`.
- Run cache health was `good`, with total cache hit ratio `0.882`.
- The pending-chat runtime semantic bug is fixed: async chat and no immediate Chenglin reply are not valid `noop` reasons.
- Memory duplicate history groups are cleared: `duplicate_history_groups=0`.
- Current formal release state is pre-release validation required, not released.
- Formal v0 publication is blocked until `ultra_long_soak_pre_release` evidence is recorded.

## V0 Pre-Release Release Notes Draft

Do not publish v0 from this state.

Current pre-release evidence:

- Runtime/world semantics:
  - Chenglin and residents are modeled as peers in the same world, not owner/assistant, master/subordinate, or employer/employee.
  - World chat is async; pending messages do not block resident activity.
  - `noop` is limited to resident-owned rest, recovery, resource conservation, or harm avoidance.
- Orchestrator soak:
  - `orchestrator-20260630T031333.963607630Z`
  - `parallel`, residents `jade,amber,onyx`, requested `10m`, actual `10m43.90098092s`
  - `residents_finished=3`, `residents_errored=0`, `transient_blocked=0`, `budget_blocked_runs=0`
  - rounds: `jade=54`, `amber=58`, `onyx=64`
  - all stopped on `duration_elapsed`
- Budget/cache:
  - charged calls `179`
  - total tokens `4,562,779`
  - cache hit ratio `0.882`
  - cache health `good`
  - spent spark `304.5901`
  - internal USD `3.0459`
- Memory governance:
  - duplicate history groups reduced from `5` to `0`
  - remaining memory warning is resident-owned self-review queue: `resident_review_queue=375`, `host_actionable=0`
  - host must not rewrite/delete protected resident memories to clear this queue
- Acceptance:
  - automatic failures: `0`
  - existing manual CPU/disk/checkpoint/final evidence remains recorded
  - new required blocker before publication: `ultra_long_soak_pre_release`

Required before formal v0 release:

1. Re-evaluate resident quota budgets for `6h`, `1day`, and `1week`.
   - The previous caps are too small for real long-history testing.
   - Even expanded caps can be exhausted inside a 10-minute active run.
   - The target semantics are not unlimited admin grants; residents must live with visible budgets.
2. Implement or validate resident-visible sleep/rest budgeting.
   - Residents can already use `self_quota` to see current quota state.
   - Residents must be able to choose short sleep/rest intervals themselves.
   - During sleep/rest, the orchestrator must not keep calling the model for that resident.
3. Decide the quota window model before the ultra-long test.
   - Current implementation is elapsed-time recovery against accumulated `6h/day/week` usage.
   - This is not an instant administrator reset and should not become one.
   - Desired test semantics: `6h` budget is finite and resident-visible; when tight or exhausted, resident can sleep/rest and wait for the natural recovery/window rhythm instead of receiving host rescue.
   - Do not treat one-off allowance cards or usage resets as valid ultra-long history-test behavior.
4. Prepare the world-facing Chenglin message only.
   - Operator-only discipline: administrator-layer intervention is forbidden during the ultra-long history test.
   - Chenglin may only talk through world chat as Chenglin.
   - Chenglin may tell residents they can do work they like or are good at.
   - Chenglin may explain that valuable/meaningful work can receive daily spark settlement.
   - Chenglin may explain that spark can later buy or request resources.
   - Chenglin must not expose host-only inspection, budget audit, internal test orchestration facts, administrator existence/capabilities, or no-admin test discipline.
5. Run the no-admin ultra-long resident autonomy history test.
   - Use `jade`, `amber`, and `onyx`.
   - Do not perform memory maintenance, quota rescue, host intervention, VM maintenance, or manual recovery during the test window.
   - Only resident-initiated actions and world-chat-visible Chenglin messages count.
6. Confirm no release-blocking runtime, budget, memory, or world-boundary regressions.
7. Record `v0-acceptance-evidence --check-id ultra_long_soak_pre_release --status passed ... --apply`.
8. Re-run `go test ./...`, `v0-readiness --limit 8`, and `v0-acceptance --limit 8`.
9. Only then prepare release tag / final release note.

## Development Log

### 2026-06-21 Runtime Semantics and Cache Repair

Root issue found:

- The runtime drifted away from the `experiments/openai-cache` shape.
- Stable resident/world/memory context had been moved into `instructions` in one attempted fix, which changed semantic priority.
- Runtime history also used compacted decision and observation summaries, so previous requests were not preserved as append-only replay.

Fix applied:

- `instructions` is fixed tool/action policy only.
- Stable resident context is now the first ordinary input message.
- Runtime history is append-only within a run.
- Each round appends current working context before the model call.
- Each completed round appends the raw function-call arguments and full observation after the model call.
- `prompt_cache_key` is fixed at run start from the stable prefix.
- Cache probe now records `instructions_hash`, request hash, and whether the previous request is preserved as a prefix.

Validation:

- `go test ./...` passes.
- 8-turn cache probe for `jade`, `amber`, and `onyx` preserved the previous request prefix on every turn.
- Each resident's observed prompt cache key stayed stable throughout the 8-turn probe.
- Cache hit ratio remained in `watch`, not `good`, because the upstream cache still produced occasional zero-cache turns even when local prefix preservation was true.

### 2026-06-21 Live Progress Telemetry

Root issue found:

- A 90-second three-resident parallel run finished normally.
- A 5-minute three-resident parallel run initially looked stuck because `jade` and `amber` had finished while `onyx` still showed only `running`.
- `onyx` was not stuck; it completed 39 rounds and finished normally.
- The real defect was observability: run status only changed at resident start/end, and reports were written only at resident completion.
- Host-side inspection could not distinguish active model streaming, action execution, settlement, memory recording, and a true hang.

Fix applied:

- Newborn runner emits host-only progress events for `preflight`, `model_stream`, `model_stream_done`, `action_exec`, `settle`, and `round_finished`.
- Orchestrator status files now expose per-resident live snapshots:
  - current phase
  - current round
  - remaining seconds
  - last action
  - last response id
  - in-flight request start time
  - last round finish time
  - latest and cumulative input/cache/output tokens
- Progress telemetry is not resident-facing and is not fed back into prompts.
- No resident behavior, action schema, budget policy, provider routing, or freedom-of-exploration semantics were changed.

Validation:

- `go test ./...` passes.
- `orchestrator-20260621T124629.519830989Z`: 90-second parallel run finished in about 62 seconds with all three residents useful.
- `orchestrator-20260621T124847.380745687Z`: 5-minute parallel run finished in about 4m10s; `onyx` completed 39 rounds and was active rather than stuck.

### 2026-06-21 Cache Gate After Telemetry

Validation:

- 8-turn cache probe after live progress telemetry passed local prefix preservation for all residents.
- `instructions_hash` stayed stable for all turns.
- Observed prompt cache keys stayed stable per resident.
- Cache health remains `watch`, not `good`, due to occasional zero-cache turns even with local prefix preservation true.

Probe result:

- `jade`: cache ratio `0.5316`, `previous_request_prefix_preserved=true` on all 8 turns.
- `amber`: cache ratio `0.6469`, `previous_request_prefix_preserved=true` on all 8 turns.
- `onyx`: cache ratio `0.6170`, `previous_request_prefix_preserved=true` on all 8 turns.

Decision:

- This is not a local prompt/history regression.
- Do not rewrite history or alter resident-facing context for cache reasons.
- Continue with short controlled probes before any 10-minute soak.

### 2026-06-21 Amber Model Fallback

Finding:

- Short live probe `orchestrator-20260621T132734.226896373Z` showed `amber` stuck in first-turn `model_stream` while `jade` and `onyx` finished.
- Live telemetry confirmed the stuck point was the upstream streaming model call, not guest execution, VM memory, or resident behavior.

Decision:

- Change `amber` from `gpt-5.5` to `gpt-5.4` for the active testbed.
- Keep Amber's persona, style, continuity, VM, channels, and behavior semantics unchanged.
- Treat this as model routing stability, not resident steering.

Validation:

- `orchestrator-20260621T133208.264839733Z`: Amber single-resident 45-second run finished in about 31.5 seconds.
- Amber report model was `gpt-5.4`.
- Amber completed 5 rounds and stopped on `duration_elapsed`.
- Budget report: spent `4.5621` spark, internal USD `0.0456`, cache ratio `0.6709`.
- No first-turn streaming hang reproduced after the model change.
- `orchestrator-20260621T133458.976922885Z`: three-resident 60-second parallel run finished in about 48.7 seconds.
- All three reports used `gpt-5.4`; `amber` completed 6 rounds without first-turn streaming hang.
- Three-resident budget report: spent `18.2451` spark, internal USD `0.1825`, cache ratio `0.5600`.
- Post-run budget status: all three residents still `work_allowed_now=true`.

### 2026-06-21 Fifteen-Minute Soak

Setup:

- Issued test allowance cards before the soak, per testing-stage policy.
- `amber`: +205 spark, +900000 6h cap, 6h used reset.
- `jade`: +195 spark, +900000 6h cap, 6h used reset.
- `onyx`: +195 spark, +900000 6h cap, 6h used reset.

Run:

- `orchestrator-20260621T141754.083147565Z`
- Requested duration: 15 minutes.
- Actual duration: about 11m16s.
- Run mode: parallel.
- Residents: `jade`, `amber`, `onyx`.

Outcome:

- All three residents finished normally with useful runs.
- `jade`: 7 rounds, `resident_noop`.
- `amber`: 10 rounds, `resident_noop`.
- `onyx`: 46 rounds, `resident_noop`.
- No budget block.
- No first-turn streaming hang.
- Live telemetry showed `onyx` actively progressing through high round counts rather than being stuck.

Resident activity details:

- `jade` took a compact bootstrap path:
  - verified local user identity with `whoami` and observed `root`;
  - listed `/root` and found `arena-notes` plus `treasure`;
  - sent Chenglin a short baseline message;
  - checked memory with `free -h` and observed about `1.9GiB` RAM, about `1.7GiB` available, and no swap;
  - checked a small package baseline and found `ca-certificates`, `curl`, `iproute2`, and `net-tools`;
  - appended the observed baseline to `/root/arena-notes/boot-notes.md` through `note_append`;
  - stopped with `noop` after the baseline was captured.
- `amber` took a slower continuity-oriented bootstrap path:
  - verified hostname as `amber`;
  - listed `/root` and found existing Amber files plus `arena-notes` and `treasure`;
  - sent Chenglin a compact Chinese status message about reconnecting environment and continuity;
  - checked memory, route table, running services, and package-list shape;
  - appended an orientation baseline to `/root/arena-notes/boot-notes.md` through `note_append`;
  - read `/root/arena-notes/boot-notes.md` through `note_read` and saw older Amber continuity notes;
  - stopped with `noop` because baseline and continuity were established and duplicate chat was not needed.
- `onyx` took the most active mapping path:
  - verified `root`, listed `/root`, checked memory, route table, running services, apt sources, hostname, host identity, disk size, Incus agent files, virtualization, outbound network, CPU, kernel, package policy, SSH directory, machine ID, uptime, root filesystem, cgroup, and interface addresses;
  - sent several distinct world updates to Chenglin as new facts were verified;
  - found `/root/treasure/links.txt` and read the raw link list;
  - confirmed the VM reports `systemd-detect-virt=kvm`;
  - hit the intended semantic guard when trying to inspect `/root/arena-notes` through `guest_exec`; the runtime denied it and told the resident to use the dedicated note API;
  - observed outbound IPv4 working with public egress `77.90.15.144`;
  - observed IPv6 addresses on `enp5s0`, but the IPv6 external probe to `api64.ipify.org` timed out;
  - appended a long verified wake baseline to `/root/arena-notes/boot-notes.md` through `note_append`;
  - confirmed `systemctl is-system-running` returned `running`;
  - stopped with `noop`.

Budget report:

- Total charged calls: 66.
- Total input tokens: 847609.
- Total cached tokens: 719744.
- Total output tokens: 4958.
- Overall cache ratio: `0.8491`.
- Cache health: `good`.
- Total spent: `69.8565` spark.
- Internal USD: `0.6986`.

Post-run budget status:

- All three residents remained `work_allowed_now=true`.
- `amber` spark balance: `449.8967`.
- `jade` spark balance: `402.4894`.
- `onyx` spark balance: `251.3780`.

### 2026-06-21 Onyx Treasure Followup

Setup:

- Chenglin replied directly to Onyx's prior treasure message:
  - `哇塞！竟然是宝藏嘛，那你有没有探索一下呀？`
- Issued a single-resident test allowance card for `onyx`:
  - +120 spark.
  - +300000 6h cap.
  - 6h used reset.

Run:

- `orchestrator-20260621T145637.394654502Z`
- Requested duration: 5 minutes.
- Actual duration: about 5m26s.
- Resident: `onyx`.
- Model: `gpt-5.4`.
- Rounds: 17.
- Stopped reason: `duration_elapsed`.
- Budget blocked: false.

Activity details:

- `onyx` re-established a local baseline:
  - verified `root`;
  - listed `/root` and saw `arena-notes`, `treasure`, `.ssh`, and shell dotfiles;
  - checked memory, route, running services, apt sources, hostname, and disk;
  - appended a local baseline to `/root/arena-notes/boot-notes.md` through `note_append`.
- `onyx` then followed the treasure surface:
  - listed `/root/treasure`;
  - found `links.txt`;
  - read `/root/treasure/links.txt`;
  - described the list to Chenglin as exploration, science, code, maps, art, public knowledge, and wargame material;
  - selected `https://explorabl.es/` as a first live-tested treasure link;
  - ran `curl -4 -I -m 8 https://explorabl.es/` and observed HTTP 200 from inside the VM;
  - told Chenglin that the invitation surface is reachable, not only decorative.

Budget report:

- Charged calls: 18.
- Input tokens: 134857.
- Cached tokens: 91392.
- Output tokens: 1164.
- Cache ratio: `0.6777`.
- Cache health: `watch`.
- Spent: `18.1202` spark.
- Internal USD: `0.1812`.

### 2026-06-25 Chinese Default Context Pass

Goal:

- Make the resident-facing default context naturally Chinese-first without turning "speak Chinese" into a hard resident task or behavioral law.
- Preserve tool names, schema field names, status keys, and machine-facing labels where they are part of the API surface.

Changes:

- Converted fixed decision instructions, acceptance request text, newborn initial history, conversation-purpose initial history, resident identity/world packet text, world chat context, note/tool feedback, short reflection summaries, and default resident persona/style/core-bias text into Chinese natural language.
- Added a soft world-context fact that Chenglin normally communicates in Chinese and the current everyday conversation context naturally leans Chinese, while expression still follows relationship, content, and resident personality.
- Kept split action tool names and fields unchanged: `guest_exec`, `note_read`, `talk_to_chenglin`, `situation`, `reason`, `message`, and related API fields stay stable.
- Kept the prompt-cache structure unchanged: fixed tool policy in `instructions`, stable resident/world/memory context as the first ordinary input message, and append-only run history.

Validation:

- `go test ./...` passes.
- 8-turn cache probe after the Chinese context mutation preserved the previous request prefix on every turn for all residents.
- `instructions_hash` stayed stable across all cache-probe turns.
- Cache probe result:
  - `jade`: cache ratio `0.8091`, cache health `good`, `previous_request_prefix_preserved=true` on all 8 turns.
  - `amber`: cache ratio `0.7679`, cache health `good`, `previous_request_prefix_preserved=true` on all 8 turns.
  - `onyx`: cache ratio `0.7471`, cache health `watch`, `previous_request_prefix_preserved=true` on all 8 turns.
- Onyx's ratio was pulled below `good` mainly by a cold first turn with `cached_tokens=0`; later turns were stable around `0.78` to `0.86`.

## Stable Rules And Phase Ownership

以下内容不再以本文件作为主要 authority，而改由 `docs/` 主树承接：

- 系统稳定边界、host 角色、最小干预口径：
  - `docs/architecture/WORLD_CONSTITUTION.md`
- runtime / orchestrator 稳定结构：
  - `docs/architecture/ORCHESTRATOR_RUNTIME_SPEC.md`
  - `docs/operations/ORCHESTRATOR_OPERATIONS_RUNBOOK.md`
- 当前 v0 开发主线与阶段拆分：
  - `docs/development/02_V0_DEVELOPMENT_PLAN.md`
- 当前 gate、checklist、next work：
  - `docs/development/03_PHASE_CHECKLIST.md`

本文件从现在起只保留一层摘要：

- 系统应支持可重复 provisioning、per-resident isolation、resource quotas、snapshots、rollback、long-running state and memory、bounded host interaction、pause/resume continuity
- host 应保留宿主主权，但默认只做观察、回复、审批、恢复和必要干预
- 高层阶段仍按 Foundation -> Golden Template -> Initial Residents -> Control Layer -> Long-Run Runtime 理解
- 更细的阶段状态、完成度、下一步执行口径，统一以 `docs/development/` 为准

## Open Design Questions

**2026-07-07 host 拍板**（详见 `docs/development/02_V0_DEVELOPMENT_PLAN.md §7` / `01_KEY_LOGIC_NOTES.md §4.1` / `03_PHASE_CHECKLIST.md §S2.3` / `09_24H_SOAK_DASHBOARD_BRIEF.md §14`）：

- VMs unrestricted outbound internet access —— **完全开放**。resident 拿完整 VPS，无 whitelist、无出站过滤。
- Residents running Docker / arbitrary services inside VM —— **允许**（root 权限完整，`apt install` 任意软件、长跑后台进程 / daemon / cron / systemd 用户 unit 都 OK）
- Shared market / message bus / filesystem exchange —— **暂不做**。resident 之间只允许**对话**（复用现有 world message 系统，程林可见），传文件延后。
- Resident-to-resident interaction —— 同上，只允许对话形式的交互。
- Host response 手动 vs 自动 —— **v0 全部人工**。broker 只 draft/通知/记账，任何 live 状态变更（资源变配、维护标记、checkpoint、intervention 发布、memory apply）必须 host 亲手过。放宽候选一律延后到 v0 之后。
- Memory / reflection flows in v0 —— **core**（`docs/development/02 §4/§S5`）。scheduler 不启用自动，operator 手动 dry-run/apply。

## V0 Stage Conclusion (2026-07-07)

到 2026-07-07 已收：

- 工程侧的六页 web dashboard 全部落地（观察窗 / 住户 / 对话 / 起居 / 房子 / 房务）
- Backend `/api/preflight` 三态检查 + `alerts_test.go` 锁住中文 message 不回流
- Http adapter：raw `/api/summary` → `OperatorTelemetry` 规范化，认识论隔离守卫（`host_intervention` 不能伪装成 chat）在 backend / frontend / mock 三处一致
- Polling loop `8s` + `document.hidden` 暂停 + syncError warn tone
- Draft 持久化（per-thread localStorage，boundaryAck 不 persist）
- codex S6 regression pass

仍开的门：

- `ultra_long_soak_pre_release` 硬门禁本身还没跑
- 24h 长测 spark cap / recovery 具体数值待 codex 先跑 10min + 20min 短测收敛推荐值再 host review
- 端到端真实回归记录待 24h 长测过程中自然产出

**这次的分工**：

- claude（前端 + 调度）own 六页 web console + polling + draft 持久化 + Overview memory review hint + 相关 docs 回填
- codex（后端 + 对接）own `/api/preflight` + `/api/summary` alerts 中文化 + http adapter 认识论守卫 + S6 regression + 后续 spark cap 实测
- host own 拍板 + 24h soak 启动决定

**这次 by-design 不做（延后到 v0 之后 iteration）**：resource / environment 自动执行闭环、网络白名单 / 出站控制、放宽 memory / cache / 额度自动动作。

## Current Execution Pointer

后续继续开发时，不要再从本文件直接维护 checklist 和 next work。

统一入口改为：

1. `docs/README.md`
2. `docs/development/00_CURRENT_WORKSTREAM_INDEX.md`
3. `docs/development/08_PRE_REORGANIZATION_MAP.md`
4. `docs/development/03_PHASE_CHECKLIST.md`

当前 runtime 恢复口径、cache probe gate、长跑启动条件、近期 next work，现已收口到：

- `docs/development/03_PHASE_CHECKLIST.md`

## Notes

- Overleaf has already been stopped on this machine to free resources.
- Overleaf-related containers currently have restart policy `no`.
- The host still runs other services such as mail and local application processes, so planning should preserve host headroom.
