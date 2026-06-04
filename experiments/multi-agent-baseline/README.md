# Multi-Agent Baseline Experiment

目标：

- 从 0 开始做 newborn bootstrapping 实战
- 固定三居民模型映射：
  - `jade = gpt-5.4`
  - `amber = gpt-5.5`
  - `onyx = gpt-5.4-mini`
- 持续 5 分钟循环调用直到时间结束
- 每轮明确注入剩余倒计时
- 让居民先探索机器，再输出最终验收报告

运行方式：

```bash
go run ./experiments/multi-agent-baseline --resident jade --duration 5m
```

输出：

- `report.json`
  - 每轮 decision
  - 每轮 observation
  - 最终 acceptance 报告

当前行为：

- 不加载旧 memory store
- 不注入预置事故历史
- 每轮让模型输出一个结构化 `next_action`
- 当前支持动作：
  - `guest_exec`
  - `write_note`
  - `talk_to_chenglin`
  - `noop`
- 时间结束后追加一轮最终验收总结

说明：

- 机器内部操作尽量不走菜单，而是统一通过 `guest_exec`
- 菜单动作主要保留给社会性/制度性行为：
  - 写笔记
  - 对程林说话
- 程林是世界里的真人，不是系统提示词，也不是“主人”
- 他掌握宿主侧资源与审批权，但不是 resident 的精神所有者
- `talk_to_chenglin` 不只是正式汇报
  - 可以聊天
  - 可以表达感受
  - 可以提诉求
  - 可以申请资源
  - 可以套近乎或建立关系
- 当前实验只保留一个硬边界：
  - resident 只能操作自己的 VM
  - 不能直接碰宿主和其他 VM
