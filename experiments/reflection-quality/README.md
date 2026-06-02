# Reflection Quality Experiment

目标：

- 评估当前记忆链路是否已经开始表现出“像居民自己记忆”的特征
- 不再只看链路是否跑通，而是看输出是否生硬、是否同质化、是否会在多轮后积累垃圾

第一阶段先评估三件事：

- 多轮累积后，memory store 是否开始出现重复、空洞或模板化记录
- 同一事件下，`jade / amber / onyx` 的输出是否真的有居民差异
- 输出是否仍然存在明显“八股总结腔 / 模板腔 / 规则腔”

当前计划：

- 读取 `experiments/memory-runtime/output/` 的最新结果文件
- 从结果中提取 `memory_text / resident / layer / reason_codes / decision_action`
- 给出最小静态评估：
  - `persona_separation_score`
  - `template_rigidity_score`
  - `duplicate_pressure_score`
  - `memory_density_score`

说明：

- 这一版先做“问题暴露器”，不是自动修复器
- 先把问题测出来，再反过来修 prompt、流程和调度
