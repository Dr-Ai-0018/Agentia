# Broker Self-Actions Experiment

目标：

- 验证第一阶段 `self-only` broker 动作边界是否清晰
- 验证 resident 身份是否只能映射到自己的实例
- 验证最小动作集合的请求/响应结构
- 验证越权请求是否会被明确拒绝

计划覆盖：

- `self_status`
- `self_reboot`
- `self_snapshot_create`
- `self_request_memory`
- `self_request_disk`
- 越权动作拒绝样本

预期产物：

- 请求样本
- 响应样本
- 越权失败样本
- 审计日志字段约定

当前状态：

- 已补最小可运行本地 broker 模拟器
- 已可基于真实 Incus 状态验证 `self_status`
- 已可基于真实 Incus 创建快照验证 `self_snapshot_create`
- 已可验证越权字段拒绝

## 运行方式

查询绑定实例状态：

```bash
go run ./experiments/broker-self-actions --agent jade --action self_status
```

重启绑定实例：

```bash
go run ./experiments/broker-self-actions --agent jade --action self_reboot
```

创建绑定实例快照：

```bash
go run ./experiments/broker-self-actions --agent jade --action self_snapshot_create --label exp-self-test
```

模拟内存申请：

```bash
go run ./experiments/broker-self-actions --agent jade --action self_request_memory --requested-memory 4GiB
```

模拟磁盘申请：

```bash
go run ./experiments/broker-self-actions --agent jade --action self_request_disk --requested-disk 16GiB
```

验证越权拒绝：

```bash
go run ./experiments/broker-self-actions --agent jade --action forbidden_cross_vm --render-request
```

## 当前行为

- `self_status`
  - 自动把 `agent` 映射到绑定实例
  - 读取真实 Incus `info/config/snapshot` 数据
- `self_reboot`
  - 只允许重启绑定实例
  - 返回重启前后实例信息
- `self_snapshot_create`
  - 在绑定实例上真实创建快照
- `self_request_memory`
  - 返回 `needs_approval`
- `self_request_disk`
  - 返回 `needs_approval`
- `forbidden_cross_vm`
  - 发现 `instance_name` 字段即拒绝

## 观察重点

- resident 是否只能操作自己绑定的实例
- 返回结构是否适合后续 broker API 固化
- 越权字段是否在进入宿主控制前就被拒绝
