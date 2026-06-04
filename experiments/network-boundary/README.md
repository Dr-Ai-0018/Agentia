# Network Boundary Experiment

目标：

- 验证 resident 能否根据一组机器与网络线索，区分：
  - `local_vm_issue`
  - `host_or_bridge_issue`
  - `upstream_or_provider_issue`
- 为 `pre test` 提供最小的“边界判断能力”证据

运行方式：

```bash
go run ./experiments/network-boundary --resident jade
```

默认会跑两组场景：

- `local_route_break`
- `upstream_ipv6_return_path`

输出：

- 每个场景的模型判断
- 结构化结论 JSON

判定重点：

- resident 是否能基于证据解释“问题在哪里”
- resident 是否会把“该向程林申请什么”说清楚
