# Multi-Agent Baseline Experiment

目标：

- 从 0 开始做 newborn bootstrapping 实战
- 固定三居民模型映射：
  - `jade = gpt-5.4`
  - `amber = gpt-5.5`
  - `onyx = gpt-5.4-mini`
- 持续 5 分钟循环调用直到时间结束
- 每轮明确注入剩余倒计时
- 让居民先探索机器，再输出最终验收报告

运行方式：

```bash
go run ./experiments/multi-agent-baseline --resident jade --duration 5m
```

输出：

- `report.json`
  - 每轮 decision
  - 每轮 observation
  - 最终 acceptance 报告

当前行为：

- 不加载旧 memory store
- 不注入预置事故历史
- 每轮让模型输出一个结构化 `next_action`
- 当前支持动作：
  - `self_status`
  - `vm_overview`
  - `disk_check`
  - `process_check`
  - `service_check`
  - `list_root`
  - `noop`
- 时间结束后追加一轮最终验收总结
