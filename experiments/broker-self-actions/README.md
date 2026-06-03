# Broker Self-Actions Experiment

目标：

- 验证第一阶段 `self-only` broker 动作边界是否清晰
- 验证 resident 身份是否只能映射到自己的实例
- 验证最小动作集合的请求/响应结构
- 验证越权请求是否会被明确拒绝

计划覆盖：

- `self_status`
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

- 目录骨架已建立
- 待补最小可运行模拟器
