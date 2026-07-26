# Ops Scripts

放宿主机运维辅助脚本。

要求：

- 默认审慎
- 关键操作需可审计
- 不写破坏性快捷脚本
- 不把凭证写进 stdout/stderr 或进程 argv

## Console Smoke

Use `scripts/ops/console-safe-smoke.sh` for console API/artifact checks.

It may read local recovery/env files, but its output is limited to safe status
facts: HTTP codes, asset paths, hash match, summary status, abandoned run IDs,
and bundle string scan results.

Do not replace it with ad hoc `curl -u ...` or `curl -H X-Arena-Console-Token: ...`
commands during launch cleanup.
