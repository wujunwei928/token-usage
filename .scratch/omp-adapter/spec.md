# Spec: omp (oh-my-pi) Adapter

Status: ready-for-human

> 来源:2026-08-16 用户目标「参考 zcode 的支持方式,增加对 oh-my-pi 的支持,信息在 ~/.omp」。调研基于本机 `~/.omp` 实测与 `omp --help`(omp v17.3.4)。本 spec 与实现同会话完成,验收证据见各 issue 的 Comments。

## Problem Statement

仓库已有 17 个 Agent Adapter(含超越上游的 zcode),未覆盖 oh-my-pi(omp,pi 的分支,本仓库作者日常使用)。诉求:omp 的 token 用量进入与其它 agent 一致的统计口径——独立子命令报表、all-report 总览、排行榜上报。omp 数据在 `~/.omp`,需先搞清哪个文件是权威用量源。

## 调研结论(事实)

### ~/.omp 目录真相

| 路径 | 内容 | 结论 |
|---|---|---|
| `agent/sessions/<project-slug>/<ts>_<session-id>.jsonl` | pi 格式会话 JSONL,assistant 消息带逐次 `usage` | ✅ 主源 |
| `agent/sessions/<project-slug>/<ts>_<session-id>/*.jsonl` | sidecar 目录内的**子会话文件**(扩展/设计子代理,自带 `type:session` 头与 usage;本机 71 个) | ✅ 并入主源,归并父会话 |
| `agent/agent.db` | `model_usage` 仅记录各模型最后使用时间;`client_usage`/`usage_history` 为聚合计数/限额快照,无逐次调用记录 | ❌ 非逐次口径 |
| `agent/history.db` | prompt 全文 FTS 历史 | ❌ 与用量无关 |

### 会话格式(与 pi 同源)

- 会话文件布局与 `~/.pi/agent/sessions` 完全一致:`<project-slug>/<started-at>_<session-id>.jsonl`,首行 `{"type":"session","version":3,...}`。
- assistant 消息 `message.usage` 字段:`input/output/cacheRead/cacheWrite/totalTokens` + `cost.total`;**omp 额外有 `reasoningTokens`**。实测 `totalTokens = input+output+cacheRead`(reasoning 已含在 output 内),pi 的解析口径无需调整,新字段宽松忽略。
- sidecar 子会话文件的目录名即父会话文件 stem(`<ts>_<parent-id>`),父归属可从目录结构推导。

## Solution(设计共识)

- 新增自包含适配器包 `internal/adapter/omp`,解析管线镜像 pi(宽松 JSON 助手、message+usage 门、totalTokens 回退、首胜去重),差异仅身份四处:数据根 `~/.omp/agent/sessions`、env `OMP_AGENT_DIR`、模型前缀 `[omp] `、去重前缀 `omp`。
- **sidecar 父归属**:文件嵌在 `<YYYY-MM-DD>T…_<id>` 目录内时,SessionID 取父会话 id(时间戳前缀判据防项目目录名含 `_` 误判),对齐 claude sidechain / zcode 父归并口径。
- **Detected** 按 zcode 方式:加载非空或 sessions 目录存在(HasData)即算。
- 报表面:daily/monthly/session(与除 claude/opencode 外的全部 agent 一致);JSON 空报表渲染 null totals(pi 家族形状);all-report 带 IncludeProjectPath。
- 集成点:`agentNames`、rosterOrder、register 包、all-spec、AgentLabel(`oh-my-pi`)、fixtures + golden、README/CONTEXT 文档。排行榜经 roster 自动纳入(ADR 0006,无需新 ADR)。

## Out of Scope

- `agent.db` 聚合表(限额/成本历史)的读取——与逐次口径不同源,双读会双算。
- omp `--profile` 隔离 profile 的 sessions 目录发现(本机无此布局,默认根优先)。
- weekly 报表(支持矩阵仅 claude/opencode 有 weekly)。
