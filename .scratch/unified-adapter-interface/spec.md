# Spec: 统一 Agent Adapter 接口

Status: ready-for-agent

> 来源:2026-08-16 improve-codebase-architecture 架构评审(候选 1)+ grilling 两轮设计会话(8 项用户确认、6 项经事实核查后按用户授权定案)。决策已落盘 ADR 0009;CONTEXT.md 新增 Detected 术语。

## Problem Statement

token-usage 支持 17 个 Agent Adapter,但它们之间只有命名约定、没有语言级契约:LoadEntries 有 4 种签名变体,16 个 adapter 各持一份 ReportKind 枚举与 SummarizeEntries 拷贝,统一报表侧 15 份 spec 文件逐份手写管道并以魔法下标注册。后果:新增一个 adapter(ADR 0006 已确认会持续发生)要碰 6+ 个文件、复制约 250 行样板;行为差异靠复制漂移固化(周起始日、session 分组、detected 语义、定价加载语义各自为政);17 个 adapter 只有 zcode 的测试打在包级入口上。"一条 Attempt 如何变成报表数字"要跨约 11 个文件,局部性对人和 AI 导航都很差。

## Solution

引入唯一的 Agent Adapter 接口(adapter/common):每个 adapter 实现同一份窄接口,报表聚合收敛为一份共享管线,per-agent 行为差异以声明式 Report Profile 表达;注册表按 agent 名注册、显式有序 roster。统一报表与排行榜 snapshot 经接口消费;codex 与 claude 以自定义 RowSource 参与(ADR 0009 记录的永久例外)。全程输出字节不变(142 个 golden 用例为验收线)。

## User Stories

1. 作为维护者,我想让 17 个 Agent Adapter 实现同一份接口,这样新增 adapter 只写一份实现而不是复制六处样板
2. 作为维护者,我想让报表聚合逻辑只有一份实现,这样修聚合 bug 不会在 16 份拷贝里漏掉一份
3. 作为新 agent 适配器作者,我想用声明式 Report Profile 表达行为差异(周起始日、session 分组、过滤顺序),这样不必先读懂 16 份既有拷贝的分歧行
4. 作为维护者,我想让注册表按 agent 名注册,这样 roster 错位在启动期暴露而不是静默错行
5. 作为 CLI 使用者,我想让重构前后输出字节一致,这样脚本与肌肉记忆不被破坏
6. 作为 qwen/zcode/opencode 用户,我想让 Detected 保持现语义(数据源存在即算),这样窗口外无数据时仍被正确标注"本机存在"
7. 作为测试者,我想有契约测试打在 Adapter 接口上,这样任何 adapter 违反契约立刻被抓,无论它有没有 golden 用例
8. 作为 codex 用户,我想让 ServiceTier 分桶定价与 LongContext 分层在重构后分毫不变,这样 codex 报表数值不受影响
9. 作为 claude 用户,我想让独立 daily 管线原样保留,这样与上游的字节级对等不动摇
10. 作为排行榜客户端维护者,我想让 snapshot 经注册表遍历全部 agent,这样新增 adapter 自动进入排行榜上报,无需改 snapshot
11. 作为维护者,我想删掉 15 个枚举翻译器与 16 套本地 ReportKind,这样 KindDaily/ReportDaily 词汇漂移消失
12. 作为代码导航者(人或 AI),我想让"Attempt→报表数字"链路只跨两个模块,这样定位问题不用在 11 个文件间跳转
13. 作为维护者,我想让 agent 专属 CLI flag 经工厂闭包注入 adapter,这样接口不随 flag 数量膨胀
14. 作为维护者,我想让 weekly 能力表达留在 CLI 命令层,这样接口对全部 agent 一视同仁(normalize 支持矩阵是权威)
15. 作为上游同步者,我想让 parser/loader 内部继续镜像 Rust crate,这样上游移植求证仍可逐文件跳转(ADR 0009 修订的 ADR 0004 边界)
16. 作为测试者,我想让无 golden 的 adapter(gemini/kimi/qwen/openclaw/zcode)获得契约测试防线,这样它们的回归不会无声通过
17. 作为维护者,我想让各 agent 定价加载语义在本次原样保留,这样字节不变风险为零(统一收口归候选 3)
18. 作为 grok 观望者,我想让 grok 维持 notImplemented 占位,这样启用决策留给未来(其死代码清理归候选 2)
19. 作为契约测试编写者,我想复用 zcode loader_test 的 fixture 模式(环境变量指向 TempDir、打包级入口),这样契约测试与现有最佳实践一致
20. 作为维护者,我想让 droid/hermes/codebuff 三个逐字节相同的三胞胎作为首个迁移试点,这样试点风险最低、模式验证最快

## Implementation Decisions

- **接口**(adapter/common):`Adapter{ Agent() string; HasData() bool; LoadEntries(LoadRequest) (LoadResult, error) }`;`LoadRequest{ Shared *core.SharedArgs; Pricing *core.PricingMap }`;`LoadResult{ Entries []core.LoadedEntry; Detected bool }`。agent 专属 flag(openclaw 路径、pi 路径、claude 项目过滤)经工厂闭包在 adapter 构造时注入,不进共享请求结构。
- **消费接口由消费者定义**(Go 惯例):all/ 定义 RowSource;codex(Groups 直通,不强行 LoadedEntry——ServiceTier/LongContext/ReasoningOutputTokens 会丢)与 claude(独立 daily 管线)提供手写实现;其余 adapter 由「Adapter + Profile」经共享管线包装而成。snapshot 侧保留既有 claude 对账管线与 codex 有损桥。
- **Report Profile 声明式**:周起始日、月起始日(默认 Sunday;zcode 双 Monday;opencode weekly Monday)、session 分组(SessionAccumulator vs SummarizeByKey 键置换)、session 过滤顺序(先聚合按 lastActivity vs 先按日期)。标题文案、totalsNullEmpty、空 rows 提示属 CLI 层展示配置,不入 profile。
- **共享枚举**:core.ReportKind(KindDaily/KindWeekly/KindMonthly/KindSession),16 套本地枚举删除。
- **注册**:按 agent 名注册 + 单一显式有序 roster;废除魔法下标;roster 中未注册名保持 notImplemented 占位(grok 现状)。
- **Detected 语义保留现状**:HasData 仅 qwen/zcode/opencode/grok;LoadResult.Detected = 条目非空 ∨ HasData;统一为一条规则属行为变更(牵动 6 个 golden 的 Detected 行),另行评审。
- **定价加载第一阶段逐 agent 保留**:工厂闭包内沿用各 spec 现行加载语义。
- **过渡策略**:adapter 包保留 5 行 SummarizeEntries 门面(本地枚举换 core.ReportKind、转发共享管线 + 自家 profile),使 CLI 层改动仅限枚举类型机械替换;门面随候选 2 一并溶解。
- **阶段**:① all/(试点 droid/hermes/codebuff)→ ② CLI 枚举机械适配(框架收敛不在本 spec)→ ③ snapshot(17 个直接调用 → 注册表遍历)。
- 遵循 ADR 0009(接口形状、codex/claude 永久例外、ADR 0004 边界修订);领域词汇以 CONTEXT.md 为准(Agent Adapter、Usage Entry、Attempt、Detected、All-Report)。

## Testing Decisions

- 好测试只测外部行为:字节级 golden(142 用例,stdout/stderr/退出码)是总验收线——现有最高接缝,全程复用,不新建。
- 新接缝仅一个:Adapter 接口的契约测试(接口即测试面),断言包级入口的外部行为:LoadEntries 产出按时间有序、Dedup Hash 唯一、四种 kind 聚合确定性、profile 生效(周/月桶边界、session 分组)。
- 先例:zcode loader_test(fixture + 环境变量指向 TempDir + 打导出的 LoadEntries);golden runner 的 fixtures 约定。
- 不测内部纯函数实现细节(现有 amp/claude/copilot 内部函数测试保持不动)。
- 每阶段验收:golden 全绿 + 契约测试全绿,才进下一阶段。

## Out of Scope

- 候选 2:CLI 命令框架三代收敛(Gen1/Gen2→Gen3 迁移、死代码删除、grok 清理)——用户另行规划
- 候选 3:计价调用纪律收口(mode 硬编码、MissingPricing 配对、定价加载统一)
- detected 语义统一(HasData ∀ agent)
- statusline、blocks、排行榜 server 端
- LoadedEntry 语义扩展(若未来欲统一 codex 管道,属新决策)
- cli/report.go(排行榜上报)与报表 report 的命名冲突分离

## Further Notes

- ADR 0009 与 CONTEXT.md 的 Detected 术语已先行落盘。
- gemini/kimi/qwen/openclaw/zcode 无 golden 用例,契约测试是它们的第一道防线;opencode-detected-all 是 HasData 语义的既有回归锚点。
- droid/hermes/codebuff 的 report.go 与 cli 命令文件逐字节相同(仅换名),试点风险最低。

## 建议的 issue 拆分(实现阶段,留待 /to-tickets)

1. adapter/common:接口 + LoadRequest/LoadResult + core.ReportKind + 共享管线 + Report Profile 类型 + 按名注册表/roster + 契约测试套件
2. 试点迁移:droid/hermes/codebuff 三胞胎(spec 声明化 + report.go 门面 + golden 护航)
3. 其余 12 个 JSONL adapter 迁移(spec 声明化 + report.go 删除/门面)
4. claude/codex 自定义 RowSource 接入(all/ 消费统一)
5. CLI 层枚举机械适配(map*Kind 换 core.ReportKind)
6. snapshot 迁移(注册表遍历 + 17 个直接调用删除)
7. 清点:魔法下标、本地枚举、翻译器全数删除;all/ 首次获得单测

## 实施修订记录(2026-08-16 评审后)

- **过渡策略提前完成**:门面在 07 中全数删除(而非保留至候选 2)——共享管线落地后门面即死代码;CLI run 闭包因此直接改调共享管线,超出"仅枚举机械替换"的原定范围。方向与 spec 一致,字节验收线不变。
- **LoadRequest 收窄**:仅 `Shared`。原设计的 `Pricing` 字段全程无接线(定价走工厂闭包),按删除测试移除;ADR 0009 已修订。
- **契约套件强化**:补齐 spec 点名的时间有序(单调)断言、Dedup Hash 唯一断言(有标识条目)、profile 周桶归属断言、Agent() 一致性断言;17 个 adapter 全部一次通过。
- **Session 行序统一为 key 排序**:共享管线 `summarizeSessionsByActivity`(`common/pipeline.go`)按 session key 排序输出,HEAD 的 opencode 为插入顺序。现有消费者展示前均按 period 重排,输出字节不变;直接消费 `SummarizeReport` 的调用方拿到的是 qwen/zcode/claude 语义的 key 序,非 opencode 历史插入序。
