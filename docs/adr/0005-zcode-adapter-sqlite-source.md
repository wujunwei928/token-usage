# zcode Adapter 以 SQLite 分析库为主数据源(而非 JSONL 扫描)

zcode Adapter 不扫描会话 JSONL,而是只读打开 `~/.zcode/cli/db/db.sqlite`,以 `model_usage` 表(一行=一次模型 API 调用尝试)为主源;早于 CLI 0.15.x 的历史(该表上线前)用同库 `message.data.tokens` 回退补齐。备选源全部否决:`agents/*/transcript.jsonl` 单文件最大 82MB 且需自行去重;`rollout/model-io-*.jsonl` 只覆盖最近几个会话;`log/*.jsonl` 的 usage 字段被脱敏为 `"[Redacted]"`。

理由:zcode 提供了 purpose-built 的用量分析库,token 五分类、模型/会话/项目维度、时间戳一应俱全,读取成本与正确性都优于解析原始转储。其余 16 个 Adapter 仍走文件扫描——本决策是"该 agent 恰好提供分析库"的个案,不是新惯例。

统计口径随主源确定:Attempt 级——每次尝试(含重试与失败,只要 usage 非零)与辅助调用(会话标题等)都计入;reasoning tokens 并入 output;子代理会话归并父会话(与 claude Adapter 对 `subagents/` 的既有语义一致)。

## Consequences

- Go 侧以只读模式(`?mode=ro`)打开 SQLite;WAL 允许与 zcode 写入并发,读取为稳定快照,重复执行结果一致。
- 解析层依赖 zcode 的 DB schema;上游 CLI 升级变更表结构(`schema_migration`)可能破坏解析,需容错探测并给出清晰报错,而非 panic 或静默错算。
- 环境变量 `ZCODE_DATA_DIR` 覆盖默认 `~/.zcode/cli`(对齐 `OPENCODE_DATA_DIR`/`QWEN_DATA_DIR`);`~/.zcode/v2` 只是配置目录,与统计无关。
- 二期回退引入 message 级粒度,与一期的 attempt 级混在同一报表中,总量近似正确但粒度不可区分。
- 无任何定价来源(GLM 走 coding-plan 订阅,库内 cost 恒 0):成本按"未知模型=0+MissingPricingModel"处理,后续经 `pricingOverrides` 补价。
