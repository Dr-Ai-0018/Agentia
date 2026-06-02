# AI Arena Plan

## Objective

Build a local multi-agent competition environment where each AI runs inside its own isolated VM, manages its own limited resources, and is periodically rewarded or penalized based on performance.

## Chosen Direction

Use:
- `Incus`
- KVM-backed `VM` instances

Do not use competitor-facing LXC containers as the primary isolation model.

## Why This Direction

The arena is not a normal trusted automation setup. Agents may:
- self-modify
- run arbitrary code
- attempt persistence
- consume excessive resources
- interfere with other agents if boundaries are weak

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
- should work if agents are not all doing heavy compute simultaneously

## Reward and Penalty Model

Base state per competitor:
- `1 vCPU`
- `2 GiB RAM`
- `16 GiB disk`

Possible reward tiers:
- Tier 1: `+0.5 to 1 vCPU`
- Tier 2: `+1 GiB RAM`
- Tier 3: `+4 to 8 GiB disk`
- Tier 4: extra execution window or priority access to shared services

Possible penalties:
- reduce RAM to `1.5 GiB` or `1 GiB`
- cap CPU more tightly
- freeze growth of disk quota
- shorten round duration

Important constraint:
- do not overcommit CPU aggressively in the first version
- keep enough host headroom to prevent one round from destabilizing the arena

## Desired Arena Properties

The system should support:
- repeatable provisioning
- per-agent isolation
- resource quotas
- snapshots
- reset or rollback
- scoring hooks
- promotion and demotion of resources
- a way to inspect logs and outputs after each round

## Implementation Phases

### Phase 1: Foundation
- install and validate `Incus`
- confirm KVM-backed VM creation works
- choose storage pool and VM image
- define naming convention for competitor VMs

### Phase 2: Golden Template
- create one base VM image
- install required packages
- create baseline user and SSH or control path
- add any common arena bootstrap logic
- snapshot the clean template

### Phase 3: Competitor Instances
- clone 3 to 4 competitor VMs from the template
- assign initial limits
- verify network isolation and connectivity rules
- verify each VM can persist its own state independently

### Phase 4: Arena Control Layer
- define round lifecycle
- define evaluation criteria
- define reward and penalty actions
- script common host-side operations
- record checkpoints before and after rounds

### Phase 5: Tournament Workflow
- launch all competitors
- run a timed round
- collect outputs
- score each AI
- adjust resources
- repeat

## Open Design Questions

- Should VMs have unrestricted outbound internet access?
- Should competitors be allowed to run Docker inside their VM?
- Should there be a shared market, message bus, or filesystem exchange area?
- What counts as "winning": profit, uptime, artifacts produced, task score, social cooperation, survival?
- How often should rewards be applied: hourly, daily, per round?
- Should penalties be reversible in the next round?

## Next Recommended Work

When resuming this project in a later conversation, the next practical step should be:

1. inspect whether `Incus` is already installed
2. if not, install it
3. verify VM launch with KVM
4. create the first base VM template
5. define exact initial limits for 3 competitor VMs

## Notes

- Overleaf has already been stopped on this machine to free resources.
- Overleaf-related containers currently have restart policy `no`.
- The host still runs other services such as mail and local application processes, so arena planning should preserve host headroom.
