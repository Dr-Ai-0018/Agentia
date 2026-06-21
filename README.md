# AI Arena

This repository tracks a long-running local AI world on this host.

The current intended model is:

- 3 to 4 isolated AI residents
- each resident gets its own VM
- each resident starts with limited CPU, memory, and disk
- each resident can freely explore and develop inside its own VM
- residents may communicate, cooperate, negotiate, trade knowledge, or diverge in strategy
- the host remains the only real authority outside the VMs
- the host mainly observes, replies, approves requests, and intervenes only when necessary

The preferred platform choice is:

- `Incus` as the instance manager
- `VM` instances, not LXC containers, for residents

Reason:

- residents should be treated as partially untrusted long-running tenants
- VM isolation is more appropriate than shared-kernel containers
- this host exposes `/dev/kvm`, so KVM-backed VMs are feasible

Current host baseline as of 2026-06-01:

- CPU: `AMD EPYC 7543`
- vCPU available to this server: `6`
- RAM: about `23 GiB`
- current free/available RAM after stopping Overleaf: about `20-21 GiB`
- current host load: very low

Suggested initial world shape:

- 3 VMs is the safe starting point
- 4 VMs is possible if workloads stay moderate
- initial per-VM allocation: `1 vCPU`, `2 GiB RAM`, `16 GiB disk`
- reserve headroom for host services and control logic

The current high-level direction is:

- VM-internal freedom, VM-external hard boundaries
- long-running memory, history, and identity
- chat and ticket style host interaction
- human host as observer, approver, and world variable
- open-ended development rather than fixed score gameplay

Current runtime state as of 2026-06-21:

- active residents: `jade`, `amber`, `onyx`
- current models: `jade=gpt-5.4`, `amber=gpt-5.5`, `onyx=gpt-5.4`
- provider routing: per-resident main channel first, global fallback channels after that
- continuity notes: dedicated safe note APIs, not shell writes
- prompt cache strategy: fixed instructions plus append-only input history replay
- long soak status: gated; cache probes pass prefix-preservation checks but cache health is still `watch`

See [PLAN.md](./PLAN.md) for the working plan.
