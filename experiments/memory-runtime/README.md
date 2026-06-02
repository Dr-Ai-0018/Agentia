# Memory Runtime Experiment

目标：

- 打通真实的记忆生成链路，而不是本地硬编码模拟
- 验证 `resident prompt + event window + layer` 是否能稳定生成可用记忆
- 验证 Responses API 流式接入、日志落盘、输出文件落盘
- 让后端能看到真实 API 请求日志

当前实现：

- 直接调用 OpenAI Responses API
- 每层记忆采用“两段式真实请求”：
  - 第一段：结构化 memory draft
  - 第二段：质量审核 / 接受或拒绝
- 支持 `short|long|permanent` 三种记忆层
- 支持 `--all-layers` / `--auto` 一次真实生成三层记忆
- 支持 `jade|amber|onyx` 三个 resident
- 支持 `baseline|busy-day|quiet-day` 三种事件场景
- 流式接收输出
- 落 `jsonl` 请求日志
- 落带完整元数据的生成结果文件

运行方式：

```bash
go run ./experiments/memory-runtime --resident amber --scenario baseline --layer short --verbose
```

一次真实生成三层：

```bash
go run ./experiments/memory-runtime --resident amber --scenario baseline --all-layers --verbose
```

常用参数：

- `--resident jade|amber|onyx`
- `--scenario baseline|busy-day|quiet-day`
- `--layer short|long|permanent`
- `--all-layers`
- `--auto`
- `--model gpt-5.4-mini`
- `--verbose`

输出：

- 日志目录：`experiments/memory-runtime/logs/`
- 结果目录：`experiments/memory-runtime/output/`

注意：

- 这是第一版真实链路，只验证“能不能真打模型并落真实记忆”
- 已经加入基础质量闸门，但还没有接入完整的长期 memory store / decay / scheduler orchestrator
- 下一步应该把它挂到真正的调度器上，而不是继续手动指定 layer
