# Spec: zcode Adapter

Status: ready-for-agent

> 来源:2026-08-15 grill-with-docs 会话(两轮调研 + 两轮决策)。调研由两个 Explore 子代理完成(zcode 数据侧 + 仓库架构侧),关键事实经代码复核(`internal/adapter/claude/paths.go` 的 subagents 语义)。本文是调研报告与设计共识的合一;实现按末节拆 issue 另行执行。

## Problem Statement

ccusage-go 已有 16 个 Agent Adapter,但未覆盖 zcode(ZCode CLI,本仓库作者日常使用)。诉求:zcode 的 token 用量进入与其它 agent 一致的统计口径——独立子命令报表、all-report 总览、token 排行榜。难点:zcode 的数据布局与其它 agent 不同(没有 `projects/` JSONL 目录结构),需要先搞清数据在哪、口径怎么定。

## 调研结论(事实)

### ~/.zcode 目录真相

`~/.zcode/v2` 是**配置目录**(共 3.7M),不含任何用量数据:`config.json`(provider/模型配置)、`bots-model-cache.v2.json`(模型目录缓存)、`coding-plan-cache.json`(订阅状态)、`telemetry-state.json`(deviceMid)、`tasks-index.sqlite`(按 workspace 的任务索引)。

真正的数据在 `~/.zcode/cli/`(333M,1357 个文件):

| 目录 | 体积 | 内容 |
|---|---|---|
| `agents/` | 237M | `sess_*/agent_*/transcript.jsonl`(单文件最大 82MB)+ metadata |
| `db/` | 43M | **db.sqlite(40M)——专用用量分析库** |
| `log/` | 16M | `zcode-YYYY-MM-DD.jsonl` 每日结构化日志 |
| `rollout/` | 480K | `model-io-sess_*.jsonl` 完整模型请求/响应转储 |
| `artifacts/`、`exec/` 等 | 16M+ | 工具产物、shell 日志,与统计无关 |

### 数据源评估

| 源 | 内容 | 覆盖 | 结论 |
|---|---|---|---|
| `db/db.sqlite` | `model_usage`(每 attempt 一行)、`turn_usage`、`session`、`message` 等 | 全部会话(分表见下) | ✅ 主源 |
| `rollout/model-io-*.jsonl` | 每次调用的完整 wire dump(含 headers) | 仅最近 3 个会话(18 行) | ❌ 覆盖不足 |
| `agents/*/transcript.jsonl` | 逐事件日志,`model_complete` 事件带 usage | 全部会话 | ❌ 单文件最大 82MB、需自行去重 |
| `log/*.jsonl` | `model.sdk.stream.completed` 的 usage 为 `"[Redacted]"` | 仅当天 | ❌ 脱敏,不可用 |

### 主源 schema:`model_usage`(一行 = 一次模型 API 调用尝试)

- **token 列**(均 INT NOT NULL DEFAULT 0):`input_tokens`、`output_tokens`、`reasoning_tokens`、`cache_creation_input_tokens`、`cache_read_input_tokens`;另有 `provider_total_tokens`(nullable)、`computed_total_tokens`。本机数据 `cache_creation` 恒 0、`reasoning` 恒 0(bigmodel 端点行为)。
- **维度列**:`session_id`、`turn_id`、`query_source`(本机分布:main_turn 701 / subagent 1291 / session_title 5 / goal_summary_title 1 / target_completion_verification 2)、`provider_id`(如 `builtin:bigmodel-coding-plan`)、`model_id`(GLM-5.3 / GLM-5.2)、`variant`、`agent`、`mode`、`task_type`、`status`(completed 1979 / error 19 / cancelled 2)、`finish_reason`。
- **去重能力**:`logical_request_id` + `attempt_index`(重试共享 logical id)——行本身唯一,统计无需额外去重。
- **时间**:`started_at` / `first_token_at` / `completed_at`,epoch 毫秒 UTC。
- **原始留存**:`raw_usage_json`(归一化 usage)、`provider_metadata_json`(Anthropic 原始格式)。
- **辅助表**:`session`(`project_id` 如 `proj_code-ai-ccusage-ccusage-go`、`directory`、`parent_id`——子代理会话指向父会话、`task_type` interactive/subagent_child/fork;本机 22 行)、`turn_usage`(每 turn 汇总)、`message`(`data` JSON 含 per-message tokens 与恒 0 的 cost)。

### 覆盖与缺口

- 数据时间范围:2026-06-19(CLI 0.14.8,GLM-5.2)~ 2026-08-15(0.16.3,GLM-5.3)。
- `model_usage` 仅覆盖 2026-08-15(该表随 CLI 0.15.x 引入;本机 1994 行,当日 input 335.2M / output 1.39M / cache_read 329.5M)。06-19 ~ 08-14 老会话的 token 只存在于 `message.data.tokens`。

### 成本

库内无任何定价;唯一 cost 字段恒 0(coding-plan 订阅制)。模型为 GLM 系(端点 `open.bigmodel.cn/api/anthropic`,Anthropic 兼容 wire format),现有 LiteLLM/models.dev 定价表查不到。

### 仓库扩展点(4 个插件位,均为既有惯例)

1. `internal/adapter/zcode/`:`paths.go`(目录解析)+ `loader.go`(解析为 `[]core.LoadedEntry`)+ `report.go`(复用 `core.Summarize*`;参照 `internal/adapter/qwen/`)
2. `internal/cli/agent_zcode.go`:`registerAgentCommand` 自动进命令树(参照 `agent_shared.go:35` 共享构建器)
3. `internal/adapter/all/spec_zcode.go`:`RegisterSpec` 追加进名册(`all/all.go:98`;追加在末尾,勿动 claude 的 index-0 fallback)
4. `internal/report/snapshot.go:137-156`:加一行 `load("zcode", ...)` 进排行榜上报

成本侧无需改动:`core.PricingMap.Find` 对未知模型给 0 并记 MissingPricingModel。

## Solution(设计共识,全部经用户确认)

| # | 决策点 | 结论 | 理由 |
|---|---|---|---|
| R1 | 本会话产出 | 调研+共识文档(本 spec + CONTEXT.md + ADR 0005/0006);实现另起 | — |
| R2 | 并入方式 | 完全并入:zcode 默认进 all-report | 与 ccusage 对齐是第一阶段目标,后续超越上游支持更多平台 |
| R3 | 成本 | tokens 先行;cost=0 + MissingPricingModel,GLM 定价后续 `pricingOverrides` 补 | 无定价来源,订阅制 |
| R4 | 排行榜 | 纳入;zcode 是独立 tool 维度,同一 user 名下 | 架构天然支持 |
| R5 | 获取方式 | 离线只读扫描 | 与全部 Adapter 一致 |
| R6 | 主源/历史 | `db.sqlite` 的 `model_usage` 为主;`message.data` 回退补 0.15.x 前历史;分两期 | ADR 0005 |
| R7 | attempt 口径 | usage 非零即计入(含重试/失败) | 真实消耗;对齐 claude 计入 API error 条目的做法 |
| R8 | 辅助调用 | 计入(session_title 等) | 口径最简:所有 API 调用都计数 |
| R9 | 子代理归属 | 归并父会话 | 对齐 claude 现行语义(`paths.go:160-167` 对 `subagents/` 返回父会话 id) |
| R10 | reasoning tokens | 并入 output | Anthropic 口径;当前恒 0,零风险 |
| R11 | 目录/env | 默认 `~/.zcode/cli`;`ZCODE_DATA_DIR` 覆盖 | 对齐 `OPENCODE_DATA_DIR`/`QWEN_DATA_DIR` |
| R12 | 报表范围 | 子命令 daily/monthly/session(weekly 仅作为聚合 Kind 供 all-report 使用);不做 blocks/statusline | 对齐其余 16 个 Adapter 的实际命令面 |

## 实现蓝图

### 一期:model_usage 直读

- `paths.go`:解析 `ZCODE_DATA_DIR`(缺省 `~/.zcode/cli`),校验 `db/db.sqlite` 存在。
- `loader.go`:`?mode=ro` 打开(WAL 允许与 zcode 并发读写,读到稳定快照);SQL 取 `model_usage` JOIN `session`(取 `directory`/`parent_id`):
  - `Timestamp` = `started_at`(ms);`Date` = 本地时区日期(与其余 Adapter 同一 `--timezone` 机制)
  - `Usage`:Input=`input_tokens`;Output=`output_tokens + reasoning_tokens`;CacheRead=`cache_read_input_tokens`;CacheCreation=`cache_creation_input_tokens`
  - `Model` = `model_id`;`SessionID` = 沿 `session.parent_id` 上溯到顶层会话;`ProjectPath` = `session.directory`,`Project` = "zcode"(agent 名,对齐 qwen 惯例)
  - cost 走 `core.CalculateCost`(当前恒 0)
- `report.go`:复用共享构建器,行为与 qwen 等 Adapter 完全一致(`--since/--until/--json/--breakdown/...`)。
- 注册三个插件位;README(中英)补 zcode 段落。

### 二期:message.data 回退

仅对没有 `model_usage` 行的会话(0.15.x 之前),从 `message.data.tokens` 还原 per-message 用量(message 级口径,与 attempt 级混排,总量近似正确)。

## Risks / Open Questions

- zcode CLI 升级可能变更 DB schema(`schema_migration` 表):解析需容错——未知 schema 时明确报错,而非 panic 或静默错算。
- GLM 定价缺失:报表与排行榜 Cost Estimate 恒 0,补价前需在 README 说明口径。
- 二期混合粒度:attempt 级与 message 级混排,当前 `LoadedEntry` 无粒度标记(如需区分,届时加查询参数,不影响总量)。
- `model_id` 大小写/别名(GLM-5.3):补定价时注意与 overrides 键匹配。
- 对账基准:实现后可用 2026-08-15 当日 input 335.2M / output 1.39M / cache_read 329.5M 验证。

## 建议的 issue 拆分(实现阶段)

1. `01-zcode-adapter-sqlite`:adapter 包 + 单测(用本机 db 采样建脱敏 fixture)
2. `02-zcode-registration`:CLI 子命令 + all spec + snapshot 一行 + README
3. `03-zcode-message-fallback`:二期回退
