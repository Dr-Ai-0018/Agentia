# AI Arena Plan

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

As of 2026-06-21:

- Three resident VMs are the active target: `jade`, `amber`, and `onyx`.
- Resident isolation remains KVM-backed VM first. LXC is not the resident isolation target.
- Starting resource envelope is `1 vCPU`, `2 GiB RAM`, and `12 GiB disk` per resident VM.
- The runtime uses split action tools instead of one large mixed schema.
- Continuity files under `/root/arena-notes` are a dedicated note-tool surface, not a shell-write surface.
- Resident model distribution is currently `jade=gpt-5.4`, `amber=gpt-5.4`, `onyx=gpt-5.4`.
- Provider routing supports per-resident main channels before global fallback channels.
- Streaming is required for live provider checks and runtime calls.
- Budget reporting can show run spend, resident balances, and cache hit ratios.

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

## Desired System Properties

The system should support:

- repeatable provisioning
- per-resident isolation
- resource quotas
- snapshots
- reset or rollback
- long-running state and memory
- bounded host interaction
- a way to inspect logs, outputs, and state over time
- pause and resume without losing resident continuity

## Host Role

The host is not just a scorekeeper. The host should be able to:

- observe resident state
- reply through chat and tickets
- approve or deny requests
- restore or revive residents after failure
- intervene only when necessary
- preserve boundaries without micromanaging day-to-day development

The host should not need to constantly direct resident development.

## Intervention Boundary

Default intervention should stay narrow.

Current intended human intervention triggers are:

- resident is effectively stuck
- resident attempts clear boundary violation
- resident becomes broadly inactive or non-advancing for too long

Outside those cases, the host should mainly observe and respond through the existing world channels.

## Implementation Phases

### Phase 1: Foundation

- install and validate `Incus`
- confirm KVM-backed VM creation works
- choose storage pool and VM image
- define naming convention for resident VMs

Status: completed for the current three-resident testbed.

### Phase 2: Golden Template

- create one base VM image
- install required packages
- create baseline user and control path
- add common bootstrap logic
- snapshot the clean template

Status: completed enough for live newborn runtime testing.

### Phase 3: Initial Residents

- clone 3 to 4 resident VMs from the template
- assign initial limits
- verify network isolation and connectivity rules
- verify each VM can persist its own state independently

Status: active for `jade`, `amber`, and `onyx`.

### Phase 4: Control Layer

- define self-only control surface
- define chat and ticket interaction paths
- define audit and public history recording
- script common host-side operations
- record checkpoints and recovery paths

Status: active; note tools, chat, tickets, self-status, self-quota, provider failover, and budget reporting are in place.

### Phase 5: Long-Run Runtime

- launch all residents
- let them persist and explore over time
- collect logs, outputs, reflections, and requests
- allow host review and selective intervention
- support pause, resume, restore, and continuity

Status: not yet ready for long soak. Cache semantics are corrected, but cache health remains `watch`; use cache probes before longer live runs.

## Open Design Questions

- Should VMs have unrestricted outbound internet access?
- Should residents be allowed to run Docker inside their VM?
- Should there be a shared market, message bus, or filesystem exchange area?
- How much resident-to-resident interaction should exist in early phases?
- Which host responses should remain manual, and which can become automated?
- Which memory and reflection flows are mandatory in v0, and which are enhancements?

## Next Recommended Work

When resuming this project in a later conversation, the next practical step should be:

1. Run another 8-turn cache probe after any prompt, packet, or history change.
2. Treat `previous_request_prefix_preserved=false` as a local bug and stop before any soak.
3. Treat stable prefix plus occasional `cached_tokens=0` as upstream cache variability, not a reason to rewrite history.
4. Keep resident-facing outside-world material as raw files only; do not attach tasks, hints, disclaimers, or steering text.
5. Do not start a long multi-resident run unless budget status, streaming provider checks, and cache probe are all acceptable.
6. After each live run, record spend, cache ratio, resident actions, and any semantic boundary failures in this plan.

## Checklist

- [x] KVM-backed VM direction chosen over resident-facing LXC.
- [x] Three resident identities active: `jade`, `amber`, `onyx`.
- [x] Per-resident provider main channel plus global fallback channel implemented.
- [x] Streaming provider checks used for the configured channels.
- [x] Split action tools implemented for runtime decisions.
- [x] Continuity notes moved to dedicated note APIs.
- [x] Guest shell denied for `/root/arena-notes` continuity read/write/edit operations.
- [x] Semantic failure feedback added for continuity-surface mistakes.
- [x] Budget status and orchestrator budget reporting implemented.
- [x] Prompt-cache runtime history rebuilt to match `openai-cache` append-only replay.
- [x] Cache probe diagnostics added for prefix preservation and instruction hash stability.
- [x] Full Go test suite passing after cache repair.
- [x] Host-side per-resident live progress telemetry added without resident-facing steering.
- [x] Re-run cache probe after host-side telemetry change.
- [x] Resident-facing default context converted to Chinese-first natural language without a hard language mandate.
- [x] Re-run cache probe after Chinese prompt/context mutation.
- [ ] Improve cache hit ratio from `watch` to consistently acceptable before long soak.
- [ ] Re-run cache probe after any resident prompt/context mutation.
- [ ] Keep long-run launch gated on budget, provider, and cache checks.

## Notes

- Overleaf has already been stopped on this machine to free resources.
- Overleaf-related containers currently have restart policy `no`.
- The host still runs other services such as mail and local application processes, so planning should preserve host headroom.
