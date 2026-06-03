# Persona Drift Experiment

目标：

- 验证 Jade / Amber / Onyx 在多轮后是否保持差异
- 验证是否出现模板腔趋同
- 验证 resident-specific 提示与记忆是否足以维持人格稳定

计划覆盖：

- 单 resident 多轮稳定性
- 三 resident 横向差异
- 模板化语言检测
- 漂移前后对比

预期产物：

- 多轮输出样本
- 差异分析摘要
- 漂移风险结论

当前状态：

- 已补真实比较器

## 运行方式

默认按固定三模型映射运行：

```bash
go run ./experiments/persona-drift
```

固定映射为：

- `jade = gpt-5.4`
- `amber = gpt-5.5`
- `onyx = gpt-5.4-mini`

如果要跑“同模型多人设”对照：

```bash
go run ./experiments/persona-drift --same-model --model gpt-5.4
```

输出目录：

```bash
go run ./experiments/persona-drift --out-dir /tmp/ai-arena-persona-drift
```

## 当前行为

- 默认对 `jade / amber / onyx` 发起真实 Responses 流式请求
- 默认使用固定三模型映射，而不是同模型
- 使用同一组三轮任务提示做横向对照
- 保留 resident-specific instructions
- 统计：
  - resident 自己的标志词命中
  - 跨 resident 串味词命中
  - banned phrase 命中
  - 开头句式重复次数
  - 每轮居民间词汇重叠度

## 观察重点

- 三个 resident 是否还像三个不同的人
- 多轮之后是否开始串味
- 是否出现固定开头、固定句式、固定腔调
- resident-specific prompt 是否足以压住模板化漂移
- 同模型控制组与多模型固定组的差异是否明显
