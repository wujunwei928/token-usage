# 15 — 简单 JSONL 适配器批 A:pi / goose / kilo / hermes / droid / codebuff / grok

**What to build:** 七个 JSONL 型 Agent Adapter:各自数据目录发现、格式解析、支持的报表集合(与参考一致);pi 的 `--pi-path` 自定义路径。

**Blocked by:** 12 — 适配器框架。

**Status:** ready-for-agent

- [ ] 七个适配器各自的目录发现与行格式解析对等(逐个对照参考实现 paths/parser)
- [ ] 每适配器支持的报表命令集合与参考一致
- [ ] 每适配器至少 daily 表格 + `--json` golden 字节一致
- [ ] `--pi-path` 生效并有 golden
- [ ] 七个适配器在 all-agent 报表中各自成行
