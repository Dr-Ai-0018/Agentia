# Recovery Loop Experiment

本实验验证 resident 在欠费和高负荷后，是否会先还清债务，再逐步恢复可工作状态。

它验证：

1. `spark` 恢复是否优先用于还债
2. 债务未清时是否继续锁死普通调用
3. 债务清空后是否重新允许普通调用
4. `6h` strain 占用是否按时间回落

## 运行

```bash
go run ./experiments/recovery-loop
```
