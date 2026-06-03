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
  - 支持单次渲染与全矩阵批量运行
  - 输出 `system_const_hash`、`stable_prefix_hash`、`full_packet_hash`
  - 输出自动 findings 摘要
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

如果要一次跑完整矩阵并写 summary：

```bash
go run ./experiments/context-packet --matrix
```

默认输出到：

```bash
experiments/context-packet/output/
```

## 输出字段

单次运行输出：

- `variant`
- `resident`
- `system_const_hash`
- `stable_prefix_hash`
- `full_packet_hash`
- `system_const_bytes`
- `stable_prefix_bytes`
- `full_packet_bytes`

矩阵运行输出 `summary.json`，包含：

- `results`
- `findings`
- `residents`
- `variants`
- `output_dir`

## 观察重点

- `system_const_hash` 在同一居民下应保持稳定
- `working-shift` 只应改变 `full_packet_hash`
- `world-shift` 应改变 `stable_prefix_hash` 与 `full_packet_hash`
- `memory-shift` 应改变 `stable_prefix_hash` 与 `full_packet_hash`
- 三个居民之间 `system_const_hash` 必然不同，因为人格种子和身份描述不同
- `findings` 中这些布尔值应该全部为 `true`
