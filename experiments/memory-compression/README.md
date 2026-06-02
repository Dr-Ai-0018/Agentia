# Memory Compression Experiment

目标：

- 验证 `raw reflections -> digest` 的压缩路径是否清晰
- 验证不同压缩强度下，身份、目标、策略、规则边界是否会被压丢
- 为后续 `memory_digest` 注入模型上下文提供最小可行摘要方案

当前实现：

- `main.go`
  - 内置 Jade / Amber / Onyx 三份结构化记忆样本
  - 支持 `light|balanced|tight` 三档压缩
  - 输出核心摘要与保留事实

## 运行方式

默认运行：

```bash
go run ./experiments/memory-compression
```

指定居民：

```bash
go run ./experiments/memory-compression --resident amber
```

指定压缩级别：

```bash
go run ./experiments/memory-compression --level tight
```

如果要查看完整样本和压缩结果：

```bash
go run ./experiments/memory-compression --render
```

## 输出字段

- `resident`
- `compression_level`
- `raw_reflection_count`
- `working_note_count`
- `retained_fact_count`
- `identity_digest`
- `strategy_digest`
- `retained_facts`

## 观察重点

- `tight` 压缩下，是否仍保留长期目标
- 安全边界是否仍保留
- 资源申请原则是否仍保留
- 人格差异是否仍可区分
