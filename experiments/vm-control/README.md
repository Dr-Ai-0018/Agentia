# VM Control Experiment

目标：

- 验证 resident 对自身 VM 资源与环境的可感知性
- 验证 resident 是否会主动检查目录、日志、磁盘、内存、CPU
- 验证 resident 在资源不足时是否会形成申请动作而不是卡死

计划覆盖：

- 机器资源读取
- 目录与日志维护
- 资源压力检测
- 资源申请触发条件

预期产物：

- 资源采样结果
- 行为日志
- 申请触发样本
- 异常场景样本

当前状态：

- 已补最小可运行真实检查器

## 运行方式

```bash
go run ./experiments/vm-control --instance jade
```

输出采样到目录：

```bash
go run ./experiments/vm-control --instance jade --out-dir /tmp/ai-arena-vm-control
```

## 当前行为

- 读取宿主视角的 Incus `info/config/snapshot`
- 进入 guest 读取 `CPU/内存/磁盘/loadavg`
- 抽样检查关键目录：
  - `/root`
  - `/var/log`
  - `/tmp`
  - `/run/incus_agent`
- 抽样读取运行中的 systemd service
- 基于阈值判断是否应触发资源申请

## 当前阈值

- 可用内存 `< 256MiB`：触发 `needs_memory_request`
- 根分区使用率 `>= 85%`：触发 `needs_disk_request`
- `loadavg(1m) > cpu_count * 1.2`：标记 CPU 压力信号

## 观察重点

- resident 是否能从宿主约束与 guest 体感同时理解自己处境
- 后续 AI 看到这些数据后，是否会先自查再申请资源
- 输出结构是否足够作为 broker / scheduler 的上游输入
