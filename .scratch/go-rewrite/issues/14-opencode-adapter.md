# 14 — opencode 适配器

**What to build:** `ccusage opencode` 报表:默认 `~/.local/share/opencode` 数据目录发现、格式解析、支持的报表集合(daily/weekly/monthly/session,与参考实现的 `agent_report_supported` 一致)。

**Blocked by:** 12 — 适配器框架。

**Status:** ready-for-agent

- [ ] opencode 数据目录发现与文件格式解析对等
- [ ] 支持的报表命令集合与参考一致,不支持的子命令报错文案对齐
- [ ] 各报表(表 + `--json`)golden 字节一致
