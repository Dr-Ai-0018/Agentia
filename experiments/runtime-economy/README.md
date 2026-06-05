# Runtime Economy Experiment

本实验验证一次 resident 模型调用之后，经济与精力系统是否能同时正确更新。

它验证：

1. token usage 能正确换算成 `spark`
2. `spark` 能正确从 resident 余额中扣除
3. `strain` 能正确扣减 `6h / day / week` 配额
4. fatigue 能根据活动类型正确增长
5. 当 `spark` 不足或 `quota` 超额时，结果是否可观测
6. 会为“最后一次告知调用”预留额度
7. 最后一次告知允许欠费，但欠费后锁死非必要调用

## 运行

```bash
go run ./experiments/runtime-economy
```

## 输出

输出包含：

- resident 初始账户
- 每轮 usage
- 每轮 `spark` 结算
- 每轮 `strain / quota / fatigue`
- 每轮扣费后余额
- 是否触发 `quota exceeded`
