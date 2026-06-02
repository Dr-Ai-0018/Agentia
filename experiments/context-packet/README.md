# Context Packet Experiment

目标：

- 验证第一阶段上下文分层是否足够清晰
- 验证 `system_const -> world_state -> memory_digest -> recent_working_context` 的组包顺序
- 验证动态内容变化时，稳定前缀 hash 是否按预期保持稳定或局部变化
- 为后续 Responses API 接入提供可重复的前缀构造基线

当前实现：

- `main.go`
  - Go 原生组包
  - 三个居民人格种子映射
  - 四层上下文拼接
  - 输出 `system_const_hash`、`stable_prefix_hash`、`full_packet_hash`
  - 支持 `baseline|world-shift|memory-shift|working-shift` 四种变体

## 运行方式

```bash
go run ./experiments/context-packet
```

指定居民：

```bash
go run ./experiments/context-packet --resident amber
```

指定上下文变体：

```bash
go run ./experiments/context-packet --variant working-shift
```

如果要直接打印完整组包内容：

```bash
go run ./experiments/context-packet --render
```

## 输出字段

每次输出一行 JSON，包含：

- `variant`
- `resident`
- `system_const_hash`
- `stable_prefix_hash`
- `full_packet_hash`
- `system_const_bytes`
- `stable_prefix_bytes`
- `full_packet_bytes`

## 观察重点

- `system_const_hash` 在同一居民下应保持稳定
- `working-shift` 只应改变 `full_packet_hash`
- `world-shift` 应改变 `stable_prefix_hash` 与 `full_packet_hash`
- `memory-shift` 应改变 `stable_prefix_hash` 与 `full_packet_hash`
- 三个居民之间 `system_const_hash` 必然不同，因为人格种子和身份描述不同
