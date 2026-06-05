# Token Ledger Experiment

本实验验证正式运营期 token 计量的最小闭环。

它验证四件事：

1. `真实 usage`、内部 `usd_cost`、内部 `strain` 是否明确分离
2. `spark` 是否正确锚定 `USD`
3. `6h / day / week` 三层额度是否正确扣减
4. fatigue 是否按 `activity_type` 正确换算

## 运行

```bash
go run ./experiments/token-ledger
```

## 输出

输出为一组样本场景，包含：

- 原始 usage
- 内部 `usd_cost`
- 居民侧 `spark` 成本
- 内部 `strain` 拆分
- quota 扣减前后
- fatigue 增量
- 是否超额

这不是最终正式系统，只是把核心计量逻辑先打成可观测实验。
