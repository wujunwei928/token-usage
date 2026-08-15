# 11 — `--jq` 管道 + debug 体系

**What to build:** `--jq <filter>` 将 JSON 输出管道给 jq 子进程并透传其输出与退出码(jq 缺失时报错文案对齐);`-d/--debug` 与 `LOG_LEVEL` 日志体系;`--debug-samples <n>`(默认 5)输出被拒绝的样本行供排查。

**Blocked by:** 07 — 作用于全报表的 JSON 输出。

**Status:** ready-for-agent

- [ ] `--jq '.'` 等过滤器输出与退出码透传,和参考实现行为一致
- [ ] 与 `--json` 组合时的行为对等( jq 接收完整 JSON)
- [ ] 系统无 jq 时报错文案字节对齐
- [ ] `-d/--debug` 与 `LOG_LEVEL`(0-5)日志级别对等
- [ ] `--debug-samples` 输出被拒绝行的样本,数量与格式对齐
