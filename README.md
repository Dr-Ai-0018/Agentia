# AI Arena

This repository tracks the plan for a multi-agent arena running on this host.

The intended model is:
- 3 to 4 isolated AI "chickens"
- each AI gets its own VM
- each VM starts with limited CPU, memory, and disk
- AIs operate for a period of time with autonomy
- AIs may cooperate, compete, trade, or specialize
- after each round, the host evaluates outcomes
- winners get more resources
- losers get fewer resources or tighter limits

The preferred platform choice is:
- `Incus` as the instance manager
- `VM` instances, not LXC containers, for the competitors

Reason:
- competitor AIs should be treated as partially untrusted tenants
- VM isolation is more appropriate than shared-kernel containers
- this host exposes `/dev/kvm`, so KVM-backed VMs are feasible

Current host baseline as of 2026-06-01:
- CPU: `AMD EPYC 7543`
- vCPU available to this server: `6`
- RAM: about `23 GiB`
- current free/available RAM after stopping Overleaf: about `20-21 GiB`
- current host load: very low

Suggested initial arena shape:
- 3 VMs is the safe starting point
- 4 VMs is possible if workloads stay moderate
- initial per-VM allocation: `1 vCPU`, `2 GiB RAM`, `16 GiB disk`
- reserve headroom for host services and control logic

See [PLAN.md](./PLAN.md) for the working plan.
