# OpenAI Cache Experiment

目标：

- 验证 Responses API 在第一阶段上下文结构下的缓存命中稳定性
- 验证 `instructions`、完整历史回放、`prompt_cache_key` 对 `cached_tokens` 的影响
- 比较 `gpt-5.4`、`gpt-5.5`、`gpt-5.4-mini`

第一阶段输出物：

- 实验脚本
- 原始日志
- 观测字段说明
- 结论文档
