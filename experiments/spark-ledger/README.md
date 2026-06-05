# Spark Ledger Experiment

本实验验证 `spark` 账户账本是否能稳定处理高精度收入与支出。

它验证：

1. 微额 `spark` 扣费不会丢失
2. 工资、奖金、奖励、扣费都能记成可审计流水
3. 高精度余额会持续正确累积
4. 不允许透支为负数

## 运行

```bash
go run ./experiments/spark-ledger
```

## 输出

输出包含：

- 当前账户余额
- 全部流水
- 每笔变动后的余额
