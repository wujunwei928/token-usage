# 13 — codex 适配器

**What to build:** `ccusage codex daily|weekly|monthly|session` 完整对等:`CODEX_HOME` 下 sessions 与 archived_sessions 的发现;turn_context/token_count 事件解析(total_token_usage / last_token_usage 的四类 Token Usage 映射);fork session 重放去重;`--speed auto|standard|fast` 影响成本。

**Blocked by:** 12 — 适配器框架。

**Status:** ready-for-agent

- [ ] `CODEX_HOME`(含默认 `~/.codex`)、sessions、archived_sessions 发现对等
- [ ] token_count 解析:total 与 last_token_usage 语义区分正确,四类 token 映射有单测
- [ ] fork session 重放去重行为与参考一致(组合 fixture golden)
- [ ] `--speed` 三取值影响成本计算
- [ ] 四报表 + `--json` 全部 golden 字节一致
