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
- Resident model distribution is currently `jade=gpt-5.4`, `amber=gpt-5.5`, `onyx=gpt-5.4`.
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
- [ ] Improve cache hit ratio from `watch` to consistently acceptable before long soak.
- [ ] Re-run cache probe after any resident prompt/context mutation.
- [ ] Keep long-run launch gated on budget, provider, and cache checks.

## Notes

- Overleaf has already been stopped on this machine to free resources.
- Overleaf-related containers currently have restart policy `no`.
- The host still runs other services such as mail and local application processes, so planning should preserve host headroom.
