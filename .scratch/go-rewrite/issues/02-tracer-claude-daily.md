# 02 — Tracer bullet:claude daily 表格报表

**What to build:** `ccusage daily` 在 fixture 上输出与参考实现字节一致的表格。一根线打穿:数据发现(CLAUDE_CONFIG_DIR / XDG / 家目录,双扫去重)→ `*.jsonl` 递归收集排序 → 行管线(`"usage":{` 预过滤、null 字段拒绝、窄结构体解析、时区化日期)→ 按日聚合成 Report → SimpleTable 核心渲染。成本列走 costUSD 展示路径(auto 模式下有 costUSD 即用),token×单价计算留给工单 03。NO_COLOR 环境下的 golden。

**Blocked by:** 01 — golden 基建。

**Status:** ready-for-agent

- [ ] happy-path fixture 的 daily 表格输出(含表头、数据行、总计行)golden 字节一致
- [ ] `CLAUDE_CONFIG_DIR` 单目录、逗号分隔多目录、指向 `projects/` 本身三种形态均可发现数据
- [ ] 无 `usage` 的行、含 null 关键字段(id/cwd/model/version/sessionId/requestId/costUSD 等)的行被静默拒绝
- [ ] `--timezone <IANA>` 改变日期切分并有对应 golden;静态二进制无系统 tzdata 也能运行(内嵌时区数据)
- [ ] 并行读取与 `--single-thread` 输出字节一致(结果顺序稳定)
- [ ] projects 目录为空时输出与参考一致的空报表
- [ ] Project 名与 Session 名提取规则对等(`chat.jsonl` 取父目录、`subagents/` 取祖父目录)
