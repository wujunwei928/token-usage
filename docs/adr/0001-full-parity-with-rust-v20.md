# 以 Rust v20 为基准的完整对等重写

参考实现 `/code/ai/ccusage/ccusage` 在 v20 已从 TypeScript 重写为 Rust workspace(npm 包仅是启动器)。我们决定 Go 版本以 Rust v20 为完整对等目标:全部 6 个报表命令、16 个 agent 适配器、`ccusage.json` 配置系统、嵌入定价快照、jq 集成,而非只移植 Claude 核心子集。理由:用户明确选择了完整对等;且 Rust 版 577 个测试与快照可直接作为 Go 版的行为一致性基准(conformance suite)。

## Considered Options

- **Claude 核心 + 可扩展架构** — 工作量约为 1/5,但 16 个 agent 数据源是 Rust 版的主体价值,后补适配器会反复触碰核心抽象。
- **最小可用版** — 放弃 blocks(5 小时计费块)与 statusline,丢失 ccusage 最有特色的两个功能。
