# 02 — zcode 进 all-report 与排行榜

**What to build:** zcode 从"只能单独看"变成"默认总览可见":裸 `ccusage` 与顶层 daily/weekly/monthly/session 等 All-Report 包含 zcode 的行与总量,`--by-agent` 可拆出 zcode;`ccusage report` 上报的 Report Snapshot 含 tool=zcode 的小时级条目,排行榜个人页出现 zcode 徽章。README(中英)补 zcode 支持说明:数据位置、`ZCODE_DATA_DIR`、成本恒 0 的口径。既有 16 个 Adapter 的输出与对等回归基线不受影响(分歧是单向增量,ADR 0006)。

**Blocked by:** 01 — zcode 子命令:直读 SQLite 用量库

**Status:** ready-for-human

- [x] 裸 `ccusage` 与顶层 `daily/weekly/monthly/session` 包含 zcode 的行与总量;`--by-agent` 拆分含 zcode
- [x] `ccusage report --dry-run` 的 snapshot 含 tool=zcode 的小时级条目;正常上报后排行榜出现 zcode(有数据时)
- [x] README.md 与 README.zh-CN.md 补 zcode 段落(数据位置、ZCODE_DATA_DIR、成本恒 0 说明)
- [x] 既有 16 个 Adapter 的输出不变:对等/golden 回归全部通过

## Comments

2026-08-15 实现完成,验收证据:

- `ccusage daily --by-agent` 总览含 zcode 行与总量;名册追加在末尾(index 16),claude 的 index-0 fallback 未动。
- `ccusage report --dry-run`:zcode 独立成 tool,11 个小时格,总量与 daily 报表 8-15 行逐项一致(340,244,608 / 1,494,524 / 334,402,176)。
- README.md 与 README.zh-CN.md 各补:parity 声明的 ADR 0006 例外、使用示例、ZCode 小节(数据位置、ZCODE_DATA_DIR、口径、补价说明)。
- `go test ./...` 全绿,含 internal/golden(343s 字节级对等)与 internal/e2e 全链路。

