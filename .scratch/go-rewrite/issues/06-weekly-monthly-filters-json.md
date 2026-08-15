# 06 — weekly / monthly 报表 + 日期过滤 + JSON 输出

**What to build:** weekly 与 monthly 两种 Report(周桶按 `--start-of-week <sunday..saturday>` 切,默认 sunday);日期过滤 `--since/--until`(YYYYMMDD 与 YYYY-MM-DD 两种写法)与 `--last <N>` 最近周期;`--order <asc|desc>`;以及全报表的 `--json` / `--no-cost` 输出(结构体字段序即输出序,默认携带 modelBreakdowns)。

**Blocked by:** 03 — 成本列需计价体系;05 — 表格需渲染器字节对等。

**Status:** ready-for-agent

- [ ] weekly / monthly 聚合 golden(表头、行、总计)
- [ ] `--start-of-week` 各取值改变周桶边界并有 golden
- [ ] `--since/--until` 两种日期写法过滤 golden
- [ ] `--last <N>` 最近 N 个周期;仅 daily/weekly/monthly 可用,session/blocks 上报错文案对齐
- [ ] `--last` 与 `--since/--until`/`--sections` 互斥,报错文案对齐
- [ ] `--order asc|desc` 行序 golden
- [ ] daily/weekly/monthly 的 `--json` 输出形状与字段顺序、`--no-cost` 剔除成本字段、modelBreakdowns 默认携带——全部字节一致
