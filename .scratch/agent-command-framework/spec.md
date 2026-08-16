# Spec: 统一 Agent 命令框架(候选 2)

Status: ready-for-agent

> 来源:2026-08-16 improve-codebase-architecture 架构评审候选 2 + grilling 决策流程(第一轮 6 项经用户确认按推荐锁定;第二轮 7 项经用户授权按事实自行定案)。决策已落盘 ADR 0010;CONTEXT.md 的 Report 词条锐化(支持矩阵为权威)。承接 ADR 0009 预留的消费者第二阶段("cli 命令树搭车")。

## Problem Statement

cli 层三代 agent 命令构建机制并存:Gen1 手写 ×4(amp/copilot/codex/opencode,各复制 40-60 行 flag/窗口样板)、Gen2 `newSimpleAgentCommand` ×7(走 pflag 解析,错误消息语义与参考 ArgParser 不同)、Gen3 框架寄居 `agent_gemini.go`(460 行通用框架 + 36 行 gemini)。droid 与 hermes 重命名后 diff 为空;四处死代码(`newAgentReportCommand`、`filterOpenCodeEntriesByDate`、`newStubCommand`/`flagError`);grok 双 `disabledInit` 冻结文件还占着矩阵/roster/显示名三个名位。cli 包零单测,codex/gemini/kimi/qwen/openclaw/zcode 六个 agent 无 golden 覆盖。

## Solution

单一框架 `internal/cli/agenttree.go`:每 agent 一份 `agentCommandSpec`(标准路径 = load + Report Profile + title + sessionMeta/totalsNullEmpty;自定义渲染走 run 逃生舱);命令树/选项解析(含 bareDefault 表达 NoOptDefVal)/窗口解析归框架。子命令集合由 `normalize.go` 的 `agentReportKinds` 支持矩阵单一来源推导。本地 agentKind 枚举删除,统一 `core.ReportKind`;JSON 形状收敛 `common.AgentReportJSON`。grok 全量删除(含 adapter 包与四处名位;config-schema.json 为上游镜像不动)。验收线:142 golden 字节不变 + 新增框架单测与命令树矩阵断言。

## User Stories

1. 作为维护者,新增 agent 命令 = 一份 spec 声明,不再复制三代样板
2. 作为维护者,给 agent 加 weekly 只改支持矩阵一处,命令树/冒号别名/报错同步
3. 作为测试者,框架单测 + agent×子命令矩阵断言覆盖六个无 golden 的 agent
4. 作为 CLI 使用者,重构前后输出字节一致(142 golden 为验收线)
5. 作为 codex 用户,`--speed` 裸 flag 仍等于 auto(NoOptDefVal 语义保留)
6. 作为 CLI 使用者,裸 `<agent>` 一律默认 daily(与参考一致;原 Gen2/opencode 的分歧行为统一,均无 golden 钉住)
7. 作为代码导航者(人或 AI),agent 命令的解析/窗口/渲染逻辑各只有一处

## Decisions(grilling 定案)

| # | 决策 | 结论 |
|---|------|------|
| Q1 | 框架宿主 | 同包拆 `agenttree.go` + `agent_report.go`,gemini 缩为普通用户文件 |
| Q2 | kinds 单一来源 | 矩阵数据化(`agentReportKinds`),normalize 与命令树同源 |
| Q3 | Gen2 七家 | 全迁,Gen2 双文件删除 |
| Q4 | Gen1 四家 | 全迁(spec 扩展 extraOptions/render);codex Groups 保留在 run 闭包 |
| Q5 | 死代码/grok | 全删含 grok(复活靠 git);config-schema.json 镜像不动 |
| Q6 | 测试面 | golden 字节不变 + 框架单测 + 命令树矩阵断言 |
| R2 | agentKind 枚举 | 删除,统一 core.ReportKind;JSON 收敛 common.AgentReportJSON |
| R2 | claude 范围 | 不纳入(blocks/statusline/config 面不同) |

## 验收清单

- [x] `go build ./...` / `go vet ./...` 通过
- [x] cli 框架单测(解析器消息、bareDefault、矩阵、命令树形状)
- [x] e2e、adapter 契约测试全绿
- [x] 142 golden 用例字节不变(629s 全量通过;沙箱无外网,定价刷新逐次走 10s 超时后回退内嵌快照,输出与 golden 逐字节一致)
- [x] ADR 0010 + CONTEXT.md Report 词条锐化
