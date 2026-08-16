# token-usage

分析 Claude Code 及其他 AI 编程 agent 的本地使用记录(JSONL 会话文件或分析数据库),按时间维度聚合 token 用量与成本,输出表格/JSON 报表。本词汇表沿用参考实现(Rust v20)的既定术语,保证两版本文档与代码可互相对照。

## Language

### 数据摄入

**Agent Adapter**:
一个 agent(如 claude、codex、opencode、zcode、omp)的数据源适配器,负责发现该 agent 的本地用量记录(JSONL 会话文件或分析数据库)、解析其私有格式、产出统一的 Usage Entry。
_Avoid_: source, provider, connector

**Usage Entry**:
一条用量记录:某时刻、某模型、某 session 的一次 API 调用(即一次 Attempt)的 token 计数(可能附带 costUSD)。
_Avoid_: log line, record, event

**Attempt**:
一次模型 API 调用尝试;同一逻辑请求的每次重试各自成一次 Attempt,token 独立计入;失败的尝试只要发生消耗即计入。
_Avoid_: retry(重试只是 Attempt 的一种)

**Auxiliary Call(辅助调用)**:
agent 自身发起、非用户回合的模型调用(如生成会话标题、目标摘要);属真实消耗,计入用量。

**Token Usage**:
一个 Usage Entry 的 token 四元组:input、output、cache creation(5m/1h 写缓存)、cache read(读缓存)。
_Avoid_: token counts, usage stats

**Reasoning Tokens(思考 token)**:
模型思考/推理阶段的 token 产出;统计口径上并入 output,不单列。

**Dedup Hash**:
`(message.id, requestId)` 二元组,用于剔除重放的重复条目;sidechain 重放按 `(message.id, 空)` 容错,同 hash 保留"最完整"的条目。

**Session**:
一次连续对话,对应 `projects/` 下的一个 JSONL 文件(stem 即 session id);子代理(sidechain/subagent)会话的用量归属其父 Session。

**Project**:
会话文件在 `projects/` 下的第一级目录名,即用户运行 agent 时的工作目录路径标识。

### 计费

**Cost Mode**:
成本来源策略。`auto`(默认):有 costUSD 用之,否则按 token 计算;`calculate`:强制按 token 计算;`display`:信任 costUSD,缺失记 0。
_Avoid_: token counting

**Pricing**:
每模型的单价表(input/output/cache create/cache read,含 >200K 分层与长上下文阈值),来源优先级:config `pricingOverrides` > 实时 LiteLLM > models.dev > 构建期内嵌快照。

**Block**:
5 小时计费窗口(可用 `--session-length` 调整)。从 session 首条目的整点起算,超窗或空闲超时即切割;空闲间隔产生灰色 gap block。
_Avoid_: billing session, window

**Burn Rate**:
Block 内的消耗速率:token/分钟、非缓存 token/分钟、成本/小时,以及到 block 结束的投影(projection)。

**Model Breakdown**:
报表行按模型维度的拆分(`--breakdown`),JSON 输出默认携带。

### 命令与输出

**Report**:
按时间维度聚合的报表命令:daily、weekly、monthly、session。各 Agent Adapter 支持的 Report 集合以支持矩阵为权威(weekly 仅 claude 与 opencode,其余为 daily/monthly/session)。
_Avoid_: view, listing

**All-Report**:
跨全部 agent 的统一报表(裸 `token-usage` 即 all-daily),支持 `--sections` 一次加载输出多报表、`--by-agent` 按 agent 拆分。

**Detected**:
All-Report 标题行 `Detected:` 列出的"本机存在数据"的 agent 集合;多数 agent 以加载出非空 Usage Entry 为准,qwen/zcode/opencode/omp 以数据源存在(HasData)即算。
_Avoid_: found, discovered

**Statusline**:
供 Claude Code 状态栏 hook 调用的单行输出模式:从 stdin 读 hook JSON,输出模型/会话成本/当日成本/block 余额/上下文占用一行表情符号摘要。
_Avoid_: status bar

**Sections**:
`--sections daily,weekly,monthly,session` 一次数据加载输出多个报表的机制。
