# Experiments

本目录用于放置 AI Arena 的预实验模块。

这些实验不是一次性临时脚本，而是正式主系统落地前的验证层。每个模块都应能单独运行、单独记录结论、单独暴露失败点。

---

## 1. 目录原则

- 一个实验模块一个子目录
- 每个模块至少包含 `README.md`
- 可执行实验默认提供 `main.go`
- 每个模块都要回答三个问题：
  - 它验证什么假设
  - 它怎么运行
  - 它的输出如何影响主系统实现

---

## 2. 当前实验分区

### A. OpenAI / Context 基础层

- `openai-cache/`
  - 验证 Responses 流式、usage、`cached_tokens`、模型差异
- `context-packet/`
  - 验证上下文组包顺序、稳定前缀、hash 漂移边界

### B. Memory 机制层

- `memory-runtime/`
  - 验证多层 memory 生成、review、promote、merge、decay
- `memory-scheduler/`
  - 验证记忆触发阈值、冷却机制、daily/micro digest
- `memory-compression/`
  - 验证 reflection -> digest 的压缩路径与损失
- `reflection-quality/`
  - 验证 reflection 文本质量与 resident 差异

### C. Broker / VM 控制层

- `broker-self-actions/`
  - 验证 `self-only` 动作边界与最小 broker 行为
- `vm-control/`
  - 验证居民对自身机器、资源、目录、日志的感知与操作
- `network-boundary/`
  - 验证居民能否区分本机问题、宿主/桥接问题、上游/provider 问题

### D. Persona / Multi-Agent 层

- `persona-drift/`
  - 验证多轮后人格是否趋同、漂移或塌缩
- `multi-agent-baseline/`
  - 建立三居民并行前的最小对照样本

---

## 3. 推荐执行顺序

第一阶段不要并行乱跑，先按依赖顺序推进：

1. `openai-cache/`
2. `context-packet/`
3. `broker-self-actions/`
4. `vm-control/`
5. `memory-scheduler/`
6. `memory-compression/`
7. `reflection-quality/`
8. `persona-drift/`
9. `multi-agent-baseline/`

说明：

- `memory-runtime/` 已有较多真实链路验证，可以视为 memory 子系统的 `v0 usable` 实验基线
- 后续正式实现应优先复用实验结论，而不是从零重想

---

## 4. 模块完成标准

一个实验模块只有同时满足下面条件，才算“可汇总进入主系统”：

- 可以独立运行
- 有清晰输入
- 有稳定输出
- 有日志或结果文件
- 有明确观察指标
- 有结论，不只是跑通

---

## 5. 当前阶段焦点

当前优先收口这四个模块：

- `openai-cache/`
- `context-packet/`
- `broker-self-actions/`
- `vm-control/`
