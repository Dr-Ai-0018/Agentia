# Doctor And Recovery

更新时间：2026-06-10

## 1. 目的

本文件定义当前 `v0` 阶段最小可用的诊断与恢复口径。

当前目标不是自动修好一切，而是先做到：

- 宿主能快速看清系统是否健康
- 持久化损坏时不要直接静默崩溃
- 关键坏态有统一观察入口

## 2. 诊断入口

当前使用：

```bash
env GOCACHE=/root/ai-arena/.cache/go-build go run ./cmd/arena-broker --mode doctor
```

输出包含：

- `ok`
- `counts`
- `residents`
- `findings`

补充一个更聚焦的 world message 扫描入口：

```bash
env GOCACHE=/root/ai-arena/.cache/go-build go run ./cmd/arena-broker --mode world-scan
```

它只返回 message 文件层面的结构问题列表。

## 3. 当前会检查什么

### 3.1 brokerstate

- resident 是否存在 `runtime-state.json`
- snapshot 是否可解析
- snapshot resident id 是否与目录一致
- snapshot version / revision 是否存在

### 3.2 memory

- resident 是否存在 memory bundle
- memory bundle 是否可解析
- 是否仍在使用 legacy array 格式
- 每个 resident 的 `history_groups / abstract_memories` 数量

### 3.3 world

- world message JSONL 是否可解析
- ticket JSON 是否可解析
- 每个 resident 的 pending chat 数量
- 每个 resident 的 open ticket 数量

### 3.4 辅助产物

- audit 文件数量
- public history 文件数量
- 遗留 `.bak` / `.tmp` 文件

## 4. 当前输出如何理解

### 4.1 `ok=true`

表示当前没有发现会阻塞基本运行的硬错误。

不表示系统已经长期稳定，只表示：

- 核心持久化文件大体可读
- 没有立刻暴露出的结构性损坏

### 4.2 `findings`

- `info`
  - 提示性信息
  - 例如历史 `.bak` 文件存在
- `warn`
  - 不一定立刻阻塞运行，但应关注
  - 例如 resident 缺少 memory bundle
- `error`
  - 当前数据已有明显损坏或不一致
  - 需要优先处理

## 5. 当前恢复语义

### 5.1 broker snapshot 损坏

当前实现已经补上最小恢复策略：

- 如果 `brokerstate/<resident>/runtime-state.json` 无法解析
- 系统会先把坏文件移动到同目录下的隔离文件
- 文件名形如：

```text
runtime-state.<timestamp>.corrupt.json
```

- 然后按 resident profile 重新初始化一个干净 engine

这意味着：

- broker 不会因为单个坏 snapshot 直接整体瘫住
- 但 resident 的 runtime 连续性会退回到 profile 初始态

所以这只是“保命恢复”，不是“无损恢复”。

### 5.2 world / memory 损坏

当前 `doctor` 只负责发现，不会自动修复。

如果发现：

- message JSONL 损坏
- ticket JSON 损坏
- memory bundle 损坏

当前口径仍然是：

1. 先保留现场
2. 先跑 `doctor`
3. 先备份原文件
4. 再决定是否手工修复或写 repair 工具

### 5.3 world message 文件隔离

当前已提供最小隔离入口：

```bash
env GOCACHE=/root/ai-arena/.cache/go-build go run ./cmd/arena-broker \
  --mode world-quarantine-message-file \
  --path .agents/world/messages/2026-06-10.jsonl \
  --reason corrupt
```

行为是：

- 把目标消息文件重命名为带时间戳的 `.bak`
- 让宿主先把明显损坏的当天消息文件移出正式读取路径
- 不自动修复内容

这依然属于保守恢复，不是无损修复。

## 6. 建议操作顺序

出现异常时，先做：

1. 运行 `doctor`
2. 记录输出
3. 判断是 `warn` 还是 `error`
4. 若是 broker snapshot 损坏，查看是否已自动生成 `.corrupt.json`
5. 再决定是否继续让 resident 运行，还是先暂停巡检

## 7. 当前限制

当前 `doctor` 还没有覆盖：

- 自动 repair
- memory compaction repair
- world JSONL 局部修复
- cross-store 一致性修复
- 长时间 crash/restart soak 检测

这些属于后续阶段。
