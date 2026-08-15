Status: ready-for-agent
Feature: ccusage-go — 以 Rust v20 为基准的完整对等 Go 重写
Plan: docs/rewrite-plan.md
ADRs: docs/adr/0001 (完整对等), 0002 (CLI 行为级对等), 0003 (字节级输出对等), 0004 (单 module 镜像 crate)

## Problem Statement

ccusage 是分析 Claude Code 等 AI 编程 agent 本地使用记录、按时间维度聚合 token 用量与成本的 CLI 工具。当前官方实现(/code/ai/ccusage/ccusage)在 v20 已重写为 Rust workspace,部分用户/团队更希望以 Go 获得单一静态二进制、熟悉的工具链与可自行演进的代码库。缺少一份把"参考 Rust v20.0.19 的全部行为"固化为可验收规格的文档,重写就会在 16 个 agent 数据格式、分层计价规则、legacy CLI 兼容层等细节上不断产生歧义与回归。

## Solution

交付一个 Go 实现的 ccusage:功能与 Rust v20.0.19 完整对等(6 个报表命令 + 16 个 agent 适配器 + `ccusage.json` 配置 + 定价体系),CLI 做到行为级对等(旗标、别名、默认值、退出码、报错文案、legacy 兼容层),输出做到字节级对等(表格含边框/对齐/ANSI/宽度截断/紧凑切换,JSON 形状与字段顺序)。正确性由提交入仓的 golden file 验证——golden 由参考 Rust 二进制生成,CI 中 Go 二进制与之字节 diff,无需 Rust 工具链。用户可在两种实现间无缝切换,脚本与别名无需改动。

## User Stories

### 核心报表(claude)

1. As a ccusage 用户, I want `ccusage daily`(及作为默认命令的裸 `ccusage`)按日聚合我的 token 用量与成本, so that 我能了解每天的花费趋势
2. As a ccusage 用户, I want `ccusage monthly` 和 `ccusage weekly` 按月/周聚合, so that 我能看到更长周期的用量模式
3. As a ccusage 用户, I want `ccusage weekly --start-of-week <monday..sunday>` 指定周起始日, so that 周报边界符合我的地区习惯
4. As a ccusage 用户, I want `ccusage session` 按 session 聚合, so that 我能看到每次对话的成本
5. As a ccusage 用户, I want `ccusage session --id <sessionId>` 查看单个 session 明细, so that 我能核对特定对话的用量
6. As a ccusage 用户, I want `--breakdown` 按模型拆分报表行(默认 JSON 携带 modelBreakdowns), so that 我能分辨不同模型的花费占比
7. As a ccusage 用户, I want `--since/--until`(YYYYMMDD 或 YYYY-MM-DD)与 `--last <N>` 过滤时段, so that 我只关注关心的日期范围
8. As a ccusage 用户, I want `--order <asc|desc>` 控制行序, so that 最新数据能排在最前
9. As a ccusage 用户, I want `--instances` 按 project 分组 daily 行, so that 多项目并行时能分别核算
10. As a ccusage 用户, I want `--project <name>` 与 `--project-aliases a=X,b=Y` 过滤/重命名项目, so that 报表里的项目名对我有意义

### blocks 与 statusline(claude 专属)

11. As a Claude Code 订阅用户, I want `ccusage blocks` 识别 5 小时计费窗口, so that 我知道当前窗口还剩多少额度
12. As a ccusage 用户, I want `blocks --session-length <hours>`、`--token-limit <n|max>`、`--active`、`--recent`, so that 我能按自己的计划模式跟踪窗口与限额
13. As a ccusage 用户, I want gap(灰色)blocks、burn rate、到窗口结束的 projection、≥80% 限额告警, so that 我能预判限额风险
14. As a Claude Code 用户, I want `ccusage statusline` 从 stdin hook JSON 输出单行状态(模型/会话成本/当日成本/block 余额/上下文占用), so that 我的编辑器状态栏实时显示成本
15. As a Claude Code 用户, I want statusline 的 `--visual-burn-rate`、`--cost-source`、`--refresh-interval`、`--cache`、`--context-low/medium-threshold` 选项, so that 状态栏行为可按喜好调校
16. As a Claude Code 用户, I want statusline 结果按 session 缓存并做活动进程信号量去重, so that 刷新间隔内不重复全量计算
17. As a 达到用量上限的 Claude Code 用户, I want usage-limit reset 时间从报错日志解析并显示, so that 我知道何时恢复

### 跨 agent

18. As a 多 agent 开发者, I want `ccusage codex daily|weekly|monthly|session` 读取 Codex 会话记录(含 fork 重放去重、`--speed auto|standard|fast`), so that 我能看到 Codex 的花费
19. As a 多 agent 开发者, I want opencode、amp、droid、codebuff、hermes、pi、goose、kilo、copilot、gemini、kimi、qwen、openclaw、grok 适配器, so that 我使用的每个 agent 都有用量报表
20. As a 多 agent 开发者, I want裸 `ccusage` 的 all-agent daily 报表与 `--by-agent` 拆分, so that 一条命令看到全部 agent 的总花费
21. As a 多 agent 开发者, I want `--sections daily,weekly,monthly,session` 一次加载输出多报表, so that 全量盘点时不必重复扫描磁盘
22. As a pi/openclaw 用户, I want `--pi-path`、`--open-claw-path` 自定义数据目录, so that 非默认安装位置也能被找到
23. As a 多机器用户, I want `CLAUDE_CONFIG_DIR`(逗号分隔、可指向 config 根或 projects 目录本身)与 `CODEX_HOME` 等 16 个 agent 目录环境变量, so that 我能指向任意数据位置

### 定价与成本

24. As a ccusage 用户, I want `--mode auto|calculate|display` 三种成本来源策略, so that 我可以在官方 costUSD 与自算 token×单价之间选择
25. As a ccusage 用户, I want LiteLLM(models.dev 兜底)实时定价 + 构建期内嵌快照离线兜底, so that 无网络环境也能得到合理成本
26. As a ccusage 用户, I want `--offline`/`CCUSAGE_OFFLINE` 跳过网络请求, so that 离线或隐私敏感环境下不外联
27. As a ccusage 用户, I want分层计价(>200K 边际分层、OpenAI 式 long-context 整单切换)、1h cache-create 2× input、fast 速度乘数与 `-fast` 模型后缀, so that 成本与实际账单一致
28. As a 自定义模型用户, I want `CCUSAGE_MODEL_ALIASES`(JSON 或 `a=b,c=d`)与 config `pricingOverrides`, so that 内部/别名模型也能正确计价
29. As a ccusage 用户, I want缺价模型被记账并告警而非中断, so that 报表总是完整产出

### 数据正确性

30. As a ccusage 用户, I want按 Dedup Hash(`(message.id, requestId)`,sidechain 容错)去重并保留"最完整"条目, so that 重放的日志不会重复计费
31. As a ccusage 用户, I want null 字段行、无 semver version 行、无 usage 行被精确拒绝, so that 脏数据不会污染统计
32. As a ccusage 用户, I want advisor iteration 子条目按其模型拆分计量, so that advisor 模型的花费不被漏记
33. As a 跨时区用户, I want `--timezone <IANA>` 与内嵌时区数据, so that 静态二进制在任何机器上日期切分一致
34. As a 大数据量用户, I want按文件大小均衡的并行读取与 `"usage":{` 行预过滤, so that 数 GB 的 JSONL 也能秒级出报表
35. As a 调试用户, I want `--single-thread`、`--debug`、`--debug-samples <n>`, so that 我能单线程排查问题并查看被拒绝的样本行

### 输出与集成

36. As a 脚本作者, I want `--json`(字段顺序稳定)与 `--no-cost`, so that 我能可靠地程序化消费数据
37. As a 脚本作者, I want `--jq <filter>`(管道给 jq 子进程并透传退出码), so that 一条命令直接得到我要的字段
38. As a 终端用户, I want与 Rust 版字节一致的表格(边框、对齐、表头蓝/ACTIVE 绿/百分比红、窄终端紧凑布局、CJK/emoji 宽度感知截断), so that 两种实现的输出可直接 diff
39. As a 管道用户, I want `--color/--no-color` 与 `NO_COLOR`/`FORCE_COLOR` 优先级一致, so that CI 日志与交互终端各得其所
40. As a 终端用户, I want `--compact` 强制窄布局, so that 小窗口下表格不折行

### CLI 兼容

41. As a 老用户, I want `ccusage codex:daily` 冒号形式被归一化为 `codex daily`, so that 我的旧脚本继续工作
42. As a 老用户, I want对已移除的 `--agent`、`--daily` 等旗标得到带迁移指引的报错, so that 我能立刻知道怎么改
43. As a 老用户, I want `-a` 在 blocks 上是 `--active`、在其他命令上按旧 `--agent` 短旗标报错, so that 历史肌肉记忆行为可预期
44. As a 任何用户, I want `--config <path>` 指定配置文件、`./.ccusage/ccusage.json` → `<claude-config-dirs>/ccusage.json` 的发现序与 CLI > env > config > 默认 的合并优先级, so that 项目级默认值可团队共享
45. As a 任何用户, I want `-h/--help` 与 `-v/-V/--version`, so that 我能随时查用法与版本

### 维护者

46. As a 维护者, I want golden file 由 Rust 二进制再生、CI 纯 Go 字节 diff, so that 对等验证不依赖 Rust 工具链也能持续运行
47. As a 维护者, I want fixture 基建(临时目录构造器 + 环境变量 guard), so that 新 agent 适配器的测试写起来一样简单
48. As a 维护者, I want GoReleaser 多平台发布与 `go install` 可装, so that 用户有正规的获取渠道
49. As a 维护者, I want内嵌定价快照有脚本化更新通道, so that 定价数据可随上游 LiteLLM 刷新
50. As a 贡献者, I want包边界与参考实现 crate 一一对应, so that 任何行为疑问都能跳到对应 Rust 代码求证

## Implementation Decisions

- **基准与对等级别**(ADR-0001/0002/0003):以 Rust v20.0.19 为完整对等基准。CLI 行为级对等:旗标名、短旗标、别名、默认值、退出码、报错文案逐字一致,含 legacy 兼容层(codex:daily 归一化、-a 上下文相关语义、已移除旗标迁移报错);仅 --help 布局用 cobra 默认样式。输出字节级对等:表格与 JSON 在相同输入、相同终端宽度、相同颜色设置下字节一致。
- **代码组织**(ADR-0004):单 Go module,包边界镜像 Rust crate:入口命令包(≈ccusage crate,含 http fetcher 与 blocks/statusline 实现)、CLI 定义包(≈ccusage-cli+parser)、核心库(≈ccusage-core:类型/计价/定价/聚合/输出)、配置包(≈ccusage-config)、终端渲染包(≈ccusage-terminal:SimpleTable、显示宽度、ANSI)、适配器目录(≈rust/adapters,含 common 与 all)、测试支撑包(≈ccusage-test-support)。
- **CLI 框架**:cobra。legacy 归一化在 cobra 感知前重写 os.Args;已移除旗标检测用自定义 flag 错误钩子;参数类型移植自参考实现的 arg 结构体,互斥校验(--last vs --since/--until/--sections)在命令实现层完成,报错文案对齐参考解析器快照。
- **依赖最小化**:直接依赖仅 cobra 与 golang.org/x/term;其余标准库:time/tzdata 内嵌时区、net/http 仅在入口层以 FetchJSON 回调注入核心库(核心库零网络、可离线单测)、encoding/json。
- **数据摄入管线**:发现(config dir 环境变量 → XDG → 家目录,双扫去重,递归收 *.jsonl 按路径排序)→ 按文件大小均衡的 goroutine 并行读取(--single-thread 退化)→ 逐行 bytes.Contains("usage":{) 预过滤 → null 字段拒绝扫描 → 窄结构体反序列化 → 时区化日期 → advisor iteration 拆分 → Dedup Hash 去重(替换优先级:非 sidechain > token 总量大 > 带 speed)。
- **适配器接口**:每个 agent 适配器实现 Name/Discover/Load/SupportedReports;非 JSONL 源(amp JSON threads、copilot OTel 导出)在适配器内转换为统一 LoadedEntry;codex 的 fork 重放去重随其适配器移植。
- **定价体系**:三份快照(LiteLLM、models.dev、fast-multiplier-overrides)以 go:embed 打入二进制并提供脚本化再生;运行时拉取(10s 超时、64MB 上限、失败仅告警、models.dev 60 秒节流);成本三模式 auto/calculate/display;分层计价按参考实现的 tiered_cost 与 long_context_threshold 语义;1h cache-create 按 2× input;speed:"fast" 乘 fast_multiplier 并派生 -fast 后缀模型;模型解析链 精确 → 别名表 → 最长模糊后缀 → 实时 models.dev → 内嵌;缺价记账并告警。
- **输出**:自研 SimpleTable 移植(制表符边框、逐列对齐、多行单元格、表头蓝/ACTIVE 绿/百分比红、<100/<120 列紧凑切换、日期压缩);ANSI 码手写以满足字节对等;JSON 以结构体字段序为输出序;--jq 管道给 jq 子进程并透传退出码。
- **blocks/statusline**:5 小时窗切割(floor_to_hour 起算、超窗或空闲超时切割、gap 灰块、active 判定)、burn rate/projection/限额告警、statusline stdin hook JSON 解析与 ${TMPDIR}/ccusage-semaphore/<session>.lock 缓存整套移植。
- **配置**:发现序与 sections(defaults、commands.*、per-agent、pricingOverrides、pi.stores)与合并优先级 CLI > env > config > 默认 照抄;提供 config-schema 再生成。
- **实现顺序**:claude 纵深优先(摄入→定价→四报表→blocks/statusline 全部验证后,再逐个复制适配器:codex → opencode → 其余 13 个),里程碑 M0~M8 见 rewrite-plan,每阶段以 golden 通过为验收。

## Testing Decisions

- **好测试的标准**:只断言外部可观察行为——进程的 stdout/stderr 字节与退出码,或纯函数的输入输出对;不断言内部结构、mock 内部协作者。
- **主接缝(唯一)**:编译后的 CLI 二进制进程边界。固定环境(TZ、NO_COLOR/FORCE_COLOR、COLUMNS、CLAUDE_CONFIG_DIR 指向 fixture;statusline 用 stdin 注入 hook JSON)执行 Go 二进制,与 golden 字节 diff。golden 由本地 Rust 二进制以相同环境生成后提交入仓,CI 不需要 Rust 工具链。覆盖矩阵:命令 × 输出模式(表格/JSON)× 关键旗标组合 × 报错路径。
- **辅助接缝(仅一处)**:核心库纯函数(分层计价、long-context 切换、fast 乘数、模型别名解析链)的表驱动单测,测试值移植自参考实现的测试用例——golden 失败时无法定位到具体计价规则,此处单测补定位能力。
- **Prior art**:参考实现 577 个测试与 apps/ccusage/test/fixtures 的 fixture 集;参考入口 crate 内嵌的行为测试(dedup 替换序、null 拒绝、时区格式化、分层计价、statusline 缓存)作为移植 conformance 清单;fs_fixture 临时目录构造器与 env guard 惯例在测试支撑包中复刻。

## Out of Scope

- npm 启动器与 6 个平台子包的分发链路(M8 之后可选里程碑,不阻塞本 spec)
- Nix 打包与 VitePress 文档站(参考实现有,Go 版不做)
- 超出 v20.0.19 基线的新功能——上游后续版本的行为变更不在本 spec 内自动跟进
- 性能微优化(mimalloc 级别);性能目标仅为"并行加载下不显著慢于参考实现"
- 任何 GUI/Web 界面、CSV 原生输出(维持"jq @csv"惯例)
- 移除或替换参考 Rust 实现本身——两版本并存,Go 版为独立仓库

## Further Notes

- 领域术语以仓库根 CONTEXT.md 词汇表为准(Cost Mode、Block、Agent Adapter、Dedup Hash、Burn Rate、All-Report、Sections 等),实现与测试命名不得使用词汇表 _Avoid_ 的同义词。
- 已知开放问题(实现中决策,不阻塞):显示宽度表自研 vs 引入 golang.org/x/text;golden 生成的 TTY 探测钉死方案;Rust/Go 浮点转字符串差异须按格式化后字符串对齐;内嵌定价快照的更新节奏。
- 实现时如对参考行为有疑问,以参考 Rust 代码与其内嵌测试为准,不猜测。
