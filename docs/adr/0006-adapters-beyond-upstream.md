# 默认纳入上游 ccusage 没有的 Adapter(zcode),接受 all-report 与上游分歧

仓库此前以与上游 ccusage(Rust v20)字节级对等为纲(ADR 0001/0002/0003)。自 zcode 起进入第二阶段:新增上游不存在的 Agent Adapter,并默认纳入 all-report(裸 `ccusage`、顶层 daily/weekly/monthly/session)与排行榜上报。理由:与 ccusage 对齐是第一阶段的目标,后续要比 ccusage 支持更多平台——增量平台的用量只有进入默认总览才有意义(用户在 2026-08-15 设计会话上确认)。

分歧是单向增量:只新增 Adapter 与其行/列,不改动既有 16 个 Adapter 的任何输出;`ccusage claude ...` 等子命令与上游的对等不受影响。

## Consequences

- 裸 `ccusage` 与顶层 all-report 的输出不再与上游 ccusage 字节一致(多出 zcode 的行与总量);对等回归基线只覆盖上游也有的命令与 agent。
- 上游新版本的移植仍以上游语义为准;增量 Adapter 不参与对等测试。
- 排行榜客户端 snapshot 会多上报一个 tool(zcode),服务端无需变更(tool 是自由维度)。
- 未来每个"超越上游"的 Adapter 默认适用本决策,不再逐个立 ADR。
