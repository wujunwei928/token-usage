# Agent Adapter 统一接口与共享报表管线(adapter/common)

17 个 Agent Adapter 此前只有命名约定、没有语言级接口:LoadEntries 有 4 种签名变体,16 个 adapter 各持一份 ReportKind 枚举与 SummarizeEntries 拷贝,all/ 侧 15 份 spec 文件逐份手写「load→detected→过滤→聚合」管道并以魔法下标注册。自本 ADR 起在 `adapter/common` 定义统一接口:`Adapter{Agent() / HasData() / LoadEntries(LoadRequest) → LoadResult{Entries, Detected}}`,`LoadRequest` 仅含 `Shared`(原设计的 `Pricing` 字段按删除测试移除,定价经工厂闭包注入,见 Consequences 评审修订);agent 专属 flag(openclaw `--open-claw-path`、pi `--pi-path`、claude `--project`)经 adapter 工厂闭包注入,不进共享请求结构——接口不随 agent flag 增长字段。注册按 agent 名 + 单一显式有序 roster,废除魔法下标。

codex 与 claude 不强行并入 LoadedEntry 管道:codex 的 ServiceTier 分桶定价、LongContext 逐事件分层、ReasoningOutputTokens 在 `core.LoadedEntry` 中无对应概念,强转必有损。两者以自定义 RowSource 参与统一消费(all/ 侧 codex 直走 Groups→Row,claude 保留独立 daily 管线);snapshot 侧保留既有显式有损桥(排行榜只需小时格,损益可接受)。16 份 SummarizeEntries 的真实行为差异(周/月起始日:zcode 双 Monday、opencode weekly Monday;session 两种分组:SessionAccumulator vs SummarizeByKey 键置换;session 过滤顺序:先聚合按 lastActivity vs 先按日期)收敛为 per-agent 声明式 Report Profile,由 common 一份共享管线消费——结构收敛、字节不变。周报能力表达留在 CLI 层(normalize.go 支持矩阵是权威),接口对全部 agent 支持 4 种 Report kind。Detected 语义保留现状(HasData 仅 qwen/zcode/opencode),统一为一条规则属行为变更,牵动 6 个 golden 的 `Detected:` 行,另行评审。

本 ADR 同时修订 ADR 0004 的镜像边界:parser/loader 内部继续逐文件镜像 Rust crate(上游移植仍以上游语义为准);报表脚手架(per-adapter report.go、all/spec、cli 命令框架)允许漂移,上游 sync 的对照单位从"文件"改为"行为"(golden 字节对比)。消费者接入分三阶段:all/ 先行 → cli 命令树(架构评审候选 2 搭车)→ report snapshot;契约测试随接口落地,接口即测试面。

## Consequences

- 新增 adapter = 实现一个接口 + 一份 Report Profile 声明;16 个 report.go 与 15 份 spec 管道删除,15 个枚举翻译器随本地枚举一起消失。
- `adapter/register` 聚合导入包(空白导入全部 entries 型 adapter)承载注册副作用:不经 all/ spec 传递引入 adapter 包的注册表消费方(如排行榜 snapshot)导入它即可;codex 按例外刻意缺席。
- 评审修订:`LoadRequest` 收窄为仅 `Shared`——定价始终经工厂闭包注入(各 agent 语义保留至候选 3),原设计的 `Pricing` 字段无任何接线,按删除测试移除;同一定价加载代码收敛为 common 的两个命名 helper(RawOffline / DisplayGated),语义选择仍在各工厂调用点。
- codex/claude 的自定义 RowSource 是永久例外而非过渡态:未来若要统一,须先扩展 LoadedEntry 语义(携带 tier/长上下文/思考 token),那是新决策。
- 与 Rust 参考的逐文件对照在报表脚手架层失效;移植求证仍可跳 parser/loader,行为疑问以 golden 为准。
- 验收线:142 个 golden 用例字节不变。gemini/kimi/qwen/openclaw/zcode 无 golden 覆盖,其回归防线是契约测试 + zcode 既有 loader_test。
- detected 若未来统一(HasData ∀ agent),`opencode-detected-all` 等 6 个 golden 将变化,须与上游增量策略(ADR 0006)一并评审。
