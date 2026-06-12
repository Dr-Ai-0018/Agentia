# Worldstate Model

更新时间：2026-06-10

## 1. 目的

本文件定义当前 `v0` 阶段 `worldstate` 的持久化与并发口径。

目标不是把它包装成完美数据库，而是明确：

- 当前实现到底依赖什么写入模型
- 哪些操作是追加写
- 哪些操作是重写
- 目前哪些并发场景仍然不安全

## 2. 当前存储结构

`worldstate` 当前主要由两部分组成：

- `world/messages/YYYY-MM-DD.jsonl`
- `world/tickets/<ticket-id>.json`

## 3. 当前写入模型

### 3.1 聊天消息

聊天消息默认采用：

- 单条 append 到当日 `jsonl`

适用路径：

- resident `-> Chenglin`
- Chenglin `-> resident`

这类写入的优点是：

- 简单
- 低开销
- 不会因为单条新消息重写整天文件

### 3.2 聊天读回执

`MarkResidentMessagesRead` 当前不是 append，而是：

- 先读全量消息
- 修改目标消息的 `read_at`
- 原子重写对应消息文件

这意味着：

- 它依赖当前进程看到的是较新的完整文件视图
- 它不适合多个写者并发修改同一消息日文件
- 当前已补入最小 CAS 守卫：若重写前发现底层文件内容已变化，会拒绝覆盖并返回冲突错误，而不是静默吞掉新增消息

### 3.3 ticket

ticket 当前采用：

- 单 ticket 单 JSON 文件
- 每次更新后原子重写该 ticket 文件

这意味着：

- ticket 与消息线程天然分离
- ticket 更新不会触碰消息 `jsonl`

## 4. 当前单写者口径

当前 `worldstate` 必须按下面假设运行：

- 同一份 `.agents/world` 默认只有一个宿主进程在执行正式写操作
- resident 不直接改写这些宿主文件
- 不支持多个独立宿主 writer 并发更新同一份 worldstate

更直白地说：

- 当前实现是“单宿主写者模型”
- 不是多写者并发安全模型

## 5. 当前明确安全的场景

### 5.1 安全场景

- 单宿主顺序追加聊天
- 单宿主顺序回复聊天
- 单宿主顺序创建/回复 ticket
- 单宿主执行聊天读回执

### 5.2 仍有风险的场景

- 两个独立进程同时重写同一日消息文件
- 两个独立进程同时修改同一 ticket
- 外部脚本直接改 `.agents/world`，同时 broker 也在写

## 6. 当前工程含义

当前阶段的正确使用方式是：

- 所有正式 world 写操作优先经过 `arena-broker`
- 不要让额外脚本并发改写 `.agents/world/messages` 或 `.agents/world/tickets`
- 如果要做 repair，先停正式 writer，再修

## 7. 后续补强方向

后续如果要提升并发安全性，优先级大致是：

1. 明确单写者锁或进程级 guard
2. 为消息重写加入版本校验或 compare-and-swap 语义
3. 增加 repair / replay 工具
4. 再考虑更复杂的多写者模型

当前 `v0` 先不默认承诺这些能力。
