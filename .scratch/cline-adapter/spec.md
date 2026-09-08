# Spec: cline Adapter

Status: ready-for-human

> 来源:2026-09-08 评审后补立(先实现后补 spec,AGENTS.md 流程要求 feature 目录先立)。磁盘格式调研以 tokscale `crates/tokscale-core/src/sessions/cline.rs` 为佐证(其 `parse_cline_cli_file` 及测试钉住同一口径),并经本仓库 golden 人工核算对账。本 spec 与实现同会话完成,验收证据见各 issue 的 Comments。

## Problem Statement

仓库已有 18 个 Agent Adapter(含超越上游的 zcode/omp),未覆盖 Cline CLI。诉求:Cline CLI 的 token 用量进入与其它 agent 一致的统计口径——独立子命令报表、all-report 总览、排行榜上报。数据在 `~/.cline/data/sessions`,需先钉住会话文件的真实格式与计量口径。

## 调研结论(事实)

### 磁盘布局

| 路径 | 内容 | 结论 |
|---|---|---|
| `~/.cline/data/sessions/<id>/<id>.messages.json` | 会话消息日志,assistant 消息逐条携带 `metrics` | ✅ 主源 |
| `~/.cline/data/sessions/<id>/<id>.json` | 会话 manifest:`session_id`/`provider`/`model`/`cwd`/`workspace_root` | ✅ 回退源 |

`CLINE_DATA_DIR` 覆盖数据根(逗号分隔多根)。

### messages.json 字段口径

- 门:仅 `role == "assistant"` 且带 `metrics` 的消息计一次 API 调用;**total 为 0 但上报了 cost 的消息保留**(零 token 也可能是真实计费调用)。
- `metrics.inputTokens` **本身已含缓存桶**:input 口径为 `inputTokens − cacheReadTokens − cacheWriteTokens`(下限 0),cache read/write 两类单独保留——总量恒等于 `inputTokens + outputTokens`,不双计。
- `metrics.cost`:非负有限数(JSON 数字或数字字符串)即 provider 报告的 costUSD;负数、NaN/Inf、不可解析视为缺失。Cost Mode `auto` 优先采用,`calculate` 恒按 token,`display` 信任之缺失记 0。
- `modelInfo.id`/`modelInfo.provider`(逐消息)更新由 manifest `model`/`provider` 播种的当前值;`ts` 缺失回退文件 mtime。
- SessionID:文件 `sessionId` → manifest `session_id` → 目录/文件 stem。
- 工作区:manifest `workspace_root` 优先于 `cwd`;分隔符归一为 `/`。

## Solution(设计共识)

- 自包含适配器包 `internal/adapter/cline`,按兄弟事实标准四文件拆分(`adapter.go`/`paths.go`/`loader.go`/`parser.go`),共享定价候选解析(`findPricedModel`)。
- **Detected** 按 zcode/omp 方式:加载非空或会话消息文件存在(HasData)即算。
- 报表面:daily/monthly/session(`SessionByActivity` + `SessionFilterAfter` profile);空 JSON 报表渲染 `totals: null`(与 pi/omp/zcode/qwen 家族一致,`totalsNullEmpty: true`);all-report 带 IncludeProjectPath。
- 集成点:`agentNames`、rosterOrder、register 包、all-spec、AgentLabel(`Cline`)、fixtures + golden、README 双语/CONTEXT 文档。排行榜经 roster 自动纳入(ADR 0006,无需新 ADR)。

## Out of Scope

- VS Code 扩展版 Cline 的 globalStorage `ui_messages.json` 格式(tokscale 另有 `parse_roo_kilo_file` 路径)——本适配器只读 Cline CLI 的 sessions 布局。
- manifest `metadata.title`(会话标题)与逐回合 turn-start 语义——报表无对应维度。
