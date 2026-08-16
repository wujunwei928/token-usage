# 05 — claude / codex 手写 RowSource 接入

**What to build:** 两个永久例外(ADR 0009)接入统一消费:codex 以 Groups 直通实现自定义 RowSource——ServiceTier 分桶定价、LongContext 逐事件分层、ReasoningOutputTokens 分毫不动;claude 保留独立 daily 管线并以自定义 RowSource 参与 all/(daily/weekly/monthly 各按既有顺序语义)。两者注册进按名注册表后,all/ 对 17 个 agent 的消费形态归一。可与 03/04 并行。

**Blocked by:** 02 — 消费路径形状(试点定型)。

**Status:** done

- [x] codex 走自定义 RowSource,报表数值与迁移前完全一致(分桶/分层/思考 token 不丢失,不强行 LoadedEntry)
- [x] claude 的 66 个 golden 全绿;daily 独立管线与 weekly/monthly「先过滤日行再分桶」顺序原样
- [x] all-report 对 17 个 agent 的输出字节不变;Detected 行为不变
- [x] codex 的事件桥(snapshot 侧)保持导出可用,为 06 留接口
