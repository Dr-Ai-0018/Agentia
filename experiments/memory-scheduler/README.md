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
  - 支持单例运行与 resident x scenario 矩阵运行
  - 输出每个事件对应的记忆调度决策
  - 为 `short_reflection / micro_digest / daily_digest / high_level_rebuild` 生成文本草稿
  - 显式区分 `instant / short / long / permanent` 四层记忆
  - 为不同层设置衰减时间
  - 在记忆临近消散时生成保留提醒
  - 在 review 时间点应用到期动作：`expired / dropped / promoted / review_due`
  - 支持自然日边界补 daily digest
  - 支持同类事件冷却与 short reflection 轮次间隔
  - 支持 `history group` 脑外证据分组与抽取标记
  - 支持结果落盘到目录

## 运行方式

默认运行：

```bash
go run ./experiments/memory-scheduler
```

指定场景：

```bash
go run ./experiments/memory-scheduler --scenario busy-day
```

指定居民：

```bash
go run ./experiments/memory-scheduler --resident onyx
```

查看完整决策流：

```bash
go run ./experiments/memory-scheduler --render
```

运行完整矩阵：

```bash
go run ./experiments/memory-scheduler --matrix
```

输出结果到目录：

```bash
go run ./experiments/memory-scheduler --matrix --out-dir /tmp/ai-arena-memory-scheduler
```

## 输出字段

- `total_events`
- `short_reflection_count`
- `micro_digest_count`
- `daily_digest_count`
- `high_level_rebuild_count`
- `categories_seen`
- `generated_memory_counts`
- `generated_layer_counts`
- `retention_alert_count`
- `retention_action_count`
- `history_group_count`
- `extracted_group_count`
- `active_memory_count`
- `expired_memory_count`
- `promoted_memory_count`
- `dropped_memory_count`

## 观察重点

- 高频事件是否会引发过多 short reflection
- 安静的一天是否仍会产生 daily digest
- 连续失败是否会触发 short reflection
- 重要事件累计后是否会触发 `micro digest`
- 同一天内是否避免重复 daily digest
- 生成的文本是否真的像“记忆”，而不只是标签堆叠
- 临近过期的记忆是否会被提醒复核
- 到期时是否真正发生“删 / 丢 / 升 / 审”动作，而不是只提醒不处理
- 是否开始体现 `history group -> abstract memory` 的双轨分离
