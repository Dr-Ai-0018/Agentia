# Phase Checklist

更新时间：2026-06-10

说明：

- `[x]` 已完成
- `[~]` 进行中或部分完成
- `[ ]` 未完成

本文件面向当前收口阶段，会持续更新。

## S0. 收口准备

- [x] 建立当前收口文档索引
- [x] 建立关键逻辑说明文档
- [x] 建立开发计划文档
- [x] 建立阶段 checklist 文档
- [x] 建立 review + 测试文档

## S1. 状态一致性与可靠性

### S1.1 brokerstate

- [x] 为 resident snapshot 增加原子写策略
- [x] 为 resident snapshot 增加版本号或 etag
- [x] 为 `prepare -> apply` 增加 stale state 防护
- [x] 为 brokerstate 增加并发场景测试
- [x] 为 brokerstate 增加损坏文件恢复策略

### S1.2 worldstate

- [x] 明确 worldstate 的单写者 / 并发模型
- [x] 降低全量重写带来的丢更新风险
- [x] 增加消息与 ticket 的一致性测试
- [x] 增加损坏文件 / 中断写入恢复方案

### S1.3 memory

- [ ] 明确 memory store 的一致性边界
- [ ] 降低整 bundle 覆盖写风险
- [ ] 补 memory repair / compaction 路径
- [ ] 增加 memory 生命周期测试

### S1.4 diagnostics

- [x] 增加状态检查命令或诊断工具
- [x] 增加 repair / doctor 文档与脚本
- [x] 增加 crash / recovery 回归测试

## S2. host control 收口

### S2.1 现有能力固化

- [x] self-only 身份边界已建立
- [x] `self_status` 已真实可用
- [x] `self_reboot` 已有真实链路
- [x] `self_snapshot` 已有真实链路
- [x] `self_restore` 已有真实链路
- [x] `ticket-roundtrip` 已真实闭环

### S2.2 待补齐能力

- [ ] ticket -> host action -> result writeback 正式闭环
- [ ] CPU 资源调整执行器
- [ ] Memory 资源调整执行器
- [ ] Disk 资源调整执行器
- [ ] 宿主侧资源变更审计与回滚口径
- [ ] baseline reset / checkpoint 命名与回收策略
- [ ] Incus inventory 与 broker facts 同步

### S2.3 网络与宿主边界

- [ ] 明确 v0 的网络策略边界
- [ ] 判断哪些网络策略需要进入正式实现
- [ ] 如果进入正式实现，补最小网络控制闭环

## S3. 长期运行 orchestrator

- [ ] 固化 resident roster / run contract
- [ ] 建立正式长期运行控制器
- [ ] 建立多 resident 并行控制
- [ ] 固化 inspection report schema
- [ ] 固化 inspection summary schema
- [ ] 补暂停、恢复、失败收敛与重试口径

## S4. host decision / settlement

### S4.1 先确认定义

- [x] 和你确认 host 定期巡检时需要看到哪些结果
- [ ] 和你确认哪些状态会触发资源或环境调整
- [ ] 和你确认资源调整频率
- [ ] 和你确认 host authority 自动化程度

### S4.2 再实现能力

- [ ] 设计 inspection summary schema
- [ ] 设计 host intervention schema
- [ ] 固化默认巡检面板字段与排序
- [ ] 实现 host decision assist
- [ ] 实现 host decision -> resource adjustment 闭环
- [ ] 实现 host decision -> environment change 闭环

## S5. memory 正式接线

- [ ] 明确 v0 必需 memory 能力范围
- [ ] 决定 scheduler 是否进入正式链路
- [ ] 决定 decay / compact / merge 哪些进入正式链路
- [ ] 接入 v0 必需 memory 调度
- [ ] 补 memory 回归与观测

## S6. 文档与最终验收

- [ ] 更新顶层 `README.md`
- [ ] 更新 `PLAN.md` 的阶段结论
- [ ] 回填 `docs/README.md` 阅读顺序
- [ ] 补 v0 operator runbook
- [ ] 补 v0 acceptance checklist
- [ ] 完成真实环境端到端回归记录
