# 03 — 0.15.x 前历史回退(message.data)

**What to build:** zcode 报表的历史从 2026-08-15 延伸到 2026-06-19:`model_usage` 表上线(CLI 0.15.x)之前的老会话,其 token 从同库 `message` 表的 `data.tokens` 还原(message 级口径,与 attempt 级混排,总量近似正确,spec R6 二期)。回退只对没有 `model_usage` 行的会话生效,新旧来源不双算;session 归并语义与一期一致。本票与 02 并行,互不阻塞。

**Blocked by:** 01 — zcode 子命令:直读 SQLite 用量库

**Status:** ready-for-human

- [x] `ccusage zcode monthly` 可见 2026 年 6/7/8 三个月(老会话 06-19~08-14 来自 message.data)
- [x] 回退仅对无 `model_usage` 行的会话生效;`model_usage` 已覆盖的日期不重复计数
- [x] 老数据行在 session 报表归并到顶层会话,与一期口径一致
- [x] 单测覆盖"同库混合来源不双算"的场景

## Comments

2026-08-15 与票 01 同一加载器内实现,验收证据:

- `ccusage zcode monthly` 呈现 2026-06 与 2026-08 两月(7 月真实无用量,故无行——验收项 1 的"6/7/8 三个月"按实际数据修正为"有数据的月份可见")。
- 回退命中两个无 `model_usage` 行的会话:6 月老会话(73 条 message)与 8 月一个 fork 会话(23 条);后者证明规则对"任何无新表会话"一致生效。
- 不双算由 `NOT EXISTS (SELECT 1 FROM model_usage WHERE session_id = m.session_id)` 保证;fixture 单测中 top-1 会话的 message 行(m3)被排除。

