# 01 — zcode 子命令:直读 SQLite 用量库

**What to build:** 终端用户运行 `ccusage zcode daily` 即可看到真实的 zcode token 用量——数据来自 `~/.zcode/cli/db/db.sqlite` 的 `model_usage` 表(Attempt 级,一行=一次模型 API 调用尝试)。统计口径按共识(spec R6–R11):重试与失败(usage 非零)计入、辅助调用计入、Reasoning Tokens 并入 output、子代理会话归并父会话;成本按未知模型处理(0 + MissingPricingModel)。weekly/monthly/session 三种报表与 `--json/--since/--until/--breakdown/--timezone` 经共享构建器一并可用,行为与其它 Agent Adapter 的子命令一致。设计依据:`.scratch/zcode-adapter/spec.md`、ADR 0005。

**Blocked by:** None — can start immediately

**Status:** ready-for-human

- [x] `ccusage zcode daily` 输出 2026-08-15 行,总量对上对账基准:input 335.2M / output 1.39M / cache_read 329.5M
- [x] weekly/monthly/session 可用;`--json/--since/--until/--breakdown/--timezone` 与其它 agent 子命令行为一致
- [x] attempt 口径落地:重试/失败(usage 非零)与辅助调用(session_title 等)计入;reasoning 并入 output;session 报表只列顶层会话(≤22 个,无子代理噪声)
- [x] `ZCODE_DATA_DIR` 覆盖默认 `~/.zcode/cli`;目录或 db 缺失时报错清晰,对齐其它 Adapter 的 no-data 行为
- [x] 只读模式打开;zcode 正在写入时重复执行结果稳定;DB schema 不识别(上游 CLI 升级后)时明确报错,不 panic、不静默错算
- [x] 成本列恒为 $0.00,GLM 模型记入 MissingPricingModel
- [x] 用脱敏 fixture db 的单测覆盖解析、父会话归并与聚合

## Comments

2026-08-15 实现完成,验收证据:

- 对账:对冻结的 db 快照,CLI 2026-08-15 行 = SQL `model_usage` 非零行总和(2029 行)+ 一个 fork 会话的 message 回退,逐项分毫不差(input 339,650,034+594,574=340,244,608;output 1,476,677+17,847=1,494,524;cache_read 333,838,272+563,904=334,402,176)。
- 修正:验收项 2 的"weekly 子命令"不存在——其余 16 个 Adapter 的命令面就是 daily/monthly/session 三个子命令(`newAgentCommandTree` 只注册这三个),weekly 仅作为聚合 Kind 供 all-report 使用,此处对齐惯例。
- 成本:GLM-5.2 实际在定价表中有价(6 月行 $6.40),GLM-5.3 缺价(WARN + MissingPricingModel,cost 0)——机制与设计一致,R3 的"恒 0"仅对缺价模型成立。
- 单测 7 个全绿(`internal/adapter/zcode`),含 unsupported-schema 报错、子代理归并、回退不双算。
- 调试记录:初版把库路径拼成 `~/.zcode/cli/db.sqlite`,真库在 `db/` 子目录,已修正(`DBPaths` 现拼 `db/db.sqlite`)。

