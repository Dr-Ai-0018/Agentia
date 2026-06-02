# OpenAI Cache Experiment

目标：

- 验证 Responses API 在第一阶段上下文结构下的缓存命中稳定性
- 验证 `instructions`、完整历史回放、`prompt_cache_key` 对 `cached_tokens` 的影响
- 比较 `gpt-5.4`、`gpt-5.5`、`gpt-5.4-mini`

当前实现：

- `main.go`
  - Go 原生 HTTP 请求
  - 流式 SSE 解析
  - 完整历史手工回放
  - 固定 `prompt_cache_key`
  - 输出 `cached_tokens`、`instructions_hash`、`input_prefix_hash`

## 运行方式

先设置环境变量：

```bash
export OPENAI_API_KEY=your_key
```

可选变量：

```bash
export OPENAI_MODEL=gpt-5.4-mini
export OPENAI_BASE_URL=https://api.openai.com/v1
export OPENAI_CACHE_TURNS=5
```

运行：

```bash
go run ./experiments/openai-cache
```

如果想边流式打印边看：

```bash
go run ./experiments/openai-cache --verbose
```

## 输出字段

每轮输出一行 JSON，包含：

- `turn`
- `model`
- `response_id`
- `x_request_id`
- `prompt_cache_key_sent`
- `prompt_cache_key_observed`
- `instructions_hash`
- `input_prefix_hash`
- `input_message_count`
- `input_tokens`
- `cached_tokens`
- `output_tokens`
- `duration_ms`
- `text`

## 第一阶段判断标准

重点关注：

- `cached_tokens` 是否连续大于 `0`
- `prompt_cache_key_observed` 是否稳定
- `instructions_hash` 是否稳定
- `input_prefix_hash` 是否按预期增长而不是异常漂移
