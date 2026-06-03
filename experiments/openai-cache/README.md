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
  - 生成独立 run 目录
  - 输出 `cached_tokens`、`instructions_hash`、`input_prefix_hash`
  - 支持单模型或多模型批量跑
  - 自动落 `jsonl` 逐轮日志和 `summary.json`

## 运行方式

默认会优先读取项目根目录 `.env`。

先编辑：

```bash
vim .env
```

至少填这三个字段：

```bash
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=your_key
OPENAI_MODEL=gpt-5.4-mini
```

也可以继续用 shell 环境变量覆盖 `.env`：

```bash
export OPENAI_CACHE_TURNS=5
export OPENAI_MODELS=gpt-5.4,gpt-5.5,gpt-5.4-mini
```

运行：

```bash
go run ./experiments/openai-cache
```

如果想边流式打印边看：

```bash
go run ./experiments/openai-cache --verbose
```

如果要一次跑多个模型：

```bash
go run ./experiments/openai-cache --models gpt-5.4,gpt-5.5,gpt-5.4-mini
```

输出默认写到：

```bash
experiments/openai-cache/output/
```

每次运行会生成一个独立 run 目录，例如：

```bash
experiments/openai-cache/output/gpt-5.4-mini-20260603T120000Z/
```

其中包含：

```bash
logs/turns.jsonl
summary.json
```

## 输出字段

逐轮日志包含：

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

`summary.json` 包含：

- `turns`
- `cache_hit_turns`
- `cache_miss_turns`
- `total_input_tokens`
- `total_cached_tokens`
- `total_output_tokens`
- `average_duration_ms`
- `observed_prompt_cache_keys`
- `instructions_hash_stable`
- `input_prefix_hash_stable_turns`
- `log_path`
- `summary_path`

## 第一阶段判断标准

重点关注：

- `cached_tokens` 是否连续大于 `0`
- `prompt_cache_key_observed` 是否稳定
- `instructions_hash` 是否稳定
- `input_prefix_hash` 是否按预期增长而不是异常漂移
- `summary.json` 中 `cache_hit_turns` 是否足以支撑后续正式接入策略
