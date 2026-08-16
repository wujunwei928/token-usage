# 统一 Agent 命令框架与支持矩阵(cli/agenttree)

三代 agent 命令构建机制(Gen1 手写 ×4、Gen2 `newSimpleAgentCommand` ×7、Gen3 寄居 `agent_gemini.go` ×5)收敛为 `internal/cli/agenttree.go` 单一框架(ADR 0009 预留的消费者第二阶段):每个 agent 声明一份 `agentCommandSpec`——标准路径只需 load + Report Profile + title 与两个渲染旗标(sessionMeta、totalsNullEmpty),自定义渲染走 run 逃生舱;命令树、选项解析(复刻参考 ArgParser 的严格性与错误消息,含 `bareDefault` 表达 pflag NoOptDefVal 语义)、`--last` 窗口解析全部由框架承载。agent 子命令集合由 `normalize.go` 的 `agentReportKinds` 支持矩阵单一来源推导(同一张矩阵服务旧冒号形式、unsupported 报错与命令树),weekly 仅 claude/opencode。cli 本地 `agentKind` 枚举删除,统一 `core.ReportKind`;agent-report JSON 形状(sessionMeta 变体)收敛进 `common.AgentReportJSON`,cli 的 agentSummaryJSON 家族与 amp/copilot 的 ReportJSON 副本删除。

grok 连同冻结代码整体删除(cli 命令、all spec、adapter 包、registry/register/矩阵/显示名四处名位):上游 20.0.19 参考二进制不含 grok,冻结移植无任何消费方,复活走 git 历史。`docs/config-schema.json` 是参考 schema 的版本锁定镜像(脚本刷新),不随手改。

行为统一(均无 golden 钉住,与其余 agent 及参考语义一致):裸 `<agent>` 一律默认 daily(原 Gen2 裸 flag 报 Unknown command、opencode 裸调用显示 help);子命令 Short 文案统一 "Show X usage grouped by …" 形;额外 flag(`--pi-path`)不再出现在 `--help` 列表(与 `--open-claw-path` 现状一致)。

## Consequences

- 新增 agent 命令 = 一份 spec 声明;`agent_shared.go`、`agent_simple3.go` 与四处死代码(`newAgentReportCommand`、`filterOpenCodeEntriesByDate`、`newStubCommand`/`flagError`)删除;Gen2 的 pflag 解析路径消失,全部 agent 走参考 ArgParser 语义。
- 验收线:142 golden 用例字节不变;cli 层新增框架单测(解析器消息、bareDefault、矩阵)与 agent×子命令树矩阵断言——这是 codex/gemini/kimi/qwen/openclaw/zcode 六个无 golden agent 的 cli 防线。
- 支持矩阵与命令树同源:给 agent 增加 weekly 只改一处(矩阵),命令树、冒号别名、unsupported 报错同步变化。
- 渲染差异是行为不是样板,留在各自 run 闭包:amp 的 Credits 表格、copilot 的空数据 stderr 提示、codebuff/goose 的 "Report - Period" 标题表格、opencode 的 entries 形状 JSON、codex 的 Groups 管线(ADR 0009 永久例外)。
- codex `--speed` 经 extraOptions + bareDefault 保留 NoOptDefVal 语义(裸 `--speed` = auto);claude 命令树(blocks/statusline/config 面)不纳入本框架。
- grok 复活路径:从 git 历史恢复 adapter 包与四处名位;若上游新版本落地 grok,移植时以新 upstream 语义为准,不复活旧冻结代码。
