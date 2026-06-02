# Memory Scheduler Experiment

目标：

- 验证第一阶段记忆调度阈值是否合理
- 验证“每轮只记录，不每轮总结”的默认策略
- 验证 short reflection / micro digest / daily digest / high-level rebuild 的触发条件
- 验证同类事件冷却机制能否减少重复总结

当前实现：

- `main.go`
  - 本地事件流模拟器
  - 内置 `baseline|busy-day|quiet-day` 三种场景
  - 输出每个事件对应的记忆调度决策
  - 支持自然日边界补 daily digest
  - 支持同类事件冷却与 short reflection 轮次间隔

## 运行方式

默认运行：

```bash
go run ./experiments/memory-scheduler
```

指定场景：

```bash
go run ./experiments/memory-scheduler --scenario busy-day
```

查看完整决策流：

```bash
go run ./experiments/memory-scheduler --render
```

## 输出字段

- `total_events`
- `short_reflection_count`
- `micro_digest_count`
- `daily_digest_count`
- `high_level_rebuild_count`
- `categories_seen`

## 观察重点

- 高频事件是否会引发过多 short reflection
- 安静的一天是否仍会产生 daily digest
- 连续失败是否会触发 short reflection
- 重要事件累计后是否会触发 `micro digest`
- 同一天内是否避免重复 daily digest
