# Spec: Token 排行榜(Token Leaderboard)

Status: ready-for-agent

## Problem Statement

用户在本地用多个 AI 编程 agent(claude、codex 等)工作,ccusage-go 已能解析本地 JSONL 日志并输出个人报表,但用量数据只停留在各自机器上。用户想要截图(ZCode Token 排行榜)那样的效果:把分散在每台设备上的 token 用量汇集起来,形成社区排行榜和个人仪表盘——看到自己在朋友/同事中的消耗排名、当日逐小时节奏、30 天趋势,以及按工具/模型/成本估算的分布。同时用户对隐私有硬性要求:代码和对话内容绝不能离开本机。

## Solution

一个「客户端上报 → 服务端聚合 → 网页展示」的三段式系统:

- **客户端**:ccusage-go 新增 `report` 子命令。复用现有 16 个 Agent Adapter 与 dedup(口径与本地 `ccusage` 报表完全一致),按客户端本地时区取出今天的 entries,聚合成 Hourly Usage((date, hour, tool, model) 五类 token 计数),连同 Device ID 一起以 Report Snapshot 全量上报;重跑命令即按 Latest-wins 覆盖当日数据。支持 `--install-timer` 安装每小时 cron。
- **服务端**:Go 单二进制(HTTP API + SSR 页面 + 价格模块)+ SQLite。收到 Report Snapshot 后按 (Device, Report Date) 先删后插落库;设备首次上报自动绑定到 User Token 所属用户(每人上限 3 台 Device)。
- **网页**:Leaderboard(按 Tool / model / city / 时间范围 / Cache 口径筛选,全员累计消耗,排名卡片带模型徽标与 Cost Estimate)、Dashboard(指标卡、当日 hour×tool 时间线、近 30 天趋势、四类分布、设备列表)、价格表页、数据说明/上榜规则页、注册登录(登录后生成 User Token 供客户端配置)。
- **成本**:服务端按公开价格表(LiteLLM 快照 + 模型家族兜底估算)统一折算 Cost Estimate,查询时现算,价格修正后历史自动重算。
- **防刷(薄)**:单设备单日超阈值打 Anomaly Flag,当日不进 Leaderboard,数据保留且本人可见;上报元数据留审计轨迹。

架构红线(ADR 0001):原始 Usage Entry 永不上传,只上报聚合后的 Hourly Usage。

## User Stories

### 客户端采集与上报

1. As a 多设备用户, I want 在每台机器上跑 `ccusage report` 一条命令就把当天用量上报, so that 不需要手工整理数据。
2. As a 用户, I want 上报内容只含小时级 token 计数(无代码、无对话、无项目路径), so that 隐私不离开本机。
3. As a 用户, I want 重跑 `ccusage report` 就修正当日数据(Latest-wins), so that 本地日志去重/修正后榜单跟着变。
4. As a 用户, I want 客户端按我本地时区切分 Report Date 与小时, so that 榜单上的"今天"和我的直觉一致。
5. As a 用户, I want 设备首次上报时自动生成并持久化 Device ID, so that 不用手工配置设备身份。
6. As a 用户, I want device.json 里存人类可读的设备标签(默认 hostname), so that 个人页能认出哪台是哪台。
7. As a 用户, I want 在 ccusage.json / 环境变量 / 命令行 flag 三处任一处配置 server 地址与 User Token, so that 脚本和手动场景都方便。
8. As a 懒用户, I want `ccusage report --install-timer` 一键安装每小时 cron, so that 不用手动跑、当日尾部用量也不丢。
9. As a 任意 agent 用户, I want 上报自动覆盖全部 16 个 agent 的日志, so that 用 codex/gemini 的量也进榜。
10. As a 用户, I want 上报成功后看到当日 token 总数与设备绑定数, so that 确认上报生效。
11. As a 离线用户, I want server 不可达时命令给出清晰报错并不产生副作用, so that 能区分"没上报"和"上报了"。

### 服务端接收与存储

12. As a 服务端, I want 校验 Bearer User Token(库存哈希不存明文), so that 泄漏数据库不等于泄漏凭证。
13. As a 服务端, I want 新 Device 首次上报自动绑定到 token 所属用户, so that 用户零配置。
14. As a 服务端, I want 每用户超过 3 台 Device 时拒绝新设备并返回明确错误, so that 上榜规则可执行。
15. As a 服务端, I want 按 (Device, Report Date) 事务性先删后插, so that Latest-wins 原子生效。
16. As a 服务端, I want 每次上报记录审计元数据(时间、条数、总 token), so that 能排查异常和展示"X 分钟前同步"。
17. As a 服务端, I want 请求体上限与按 token 限速, so that 恶意大包和刷请求打不垮服务。
18. As a 服务端, I want 字段类型严格校验、非法请求 4xx, so that 脏数据不进库。

### Leaderboard

19. As a 社区成员, I want 按时间范围筛选(今天/昨天/前天/近3/近7/近30/全部), so that 看单日爆发也看长期积累。
20. As a 社区成员, I want 按 Tool 筛选(如只看 claude 或 codex), so that 不同工具的用户能分榜竞争。
21. As a 社区成员, I want 按 model 筛选, so that 同一模型下比较才有意义。
22. As a 社区成员, I want 按 city 筛选, so that 看同城排名。
23. As a 社区成员, I want 切换 Cache 口径(含缓存/仅新增), so that 缓存重度用户和轻度用户都能找到公平视角。
24. As a 社区成员, I want 看到所选范围内全员累计消耗与参与人数, so that 感受社区总活跃度。
25. As a 社区成员, I want 排名卡片显示头像、昵称、城市、模型徽标、token 数、Cost Estimate、设备数, so that 一眼读懂每个名次。
26. As a 落榜者, I want 自己在范围外/被标记时仍能从直链看自己的 Dashboard, so that 数据不会"消失"。
27. As a 访客, I want 不登录也能看 Leaderboard / 价格表 / 数据说明, so that 零门槛围观。

### Dashboard(个人仪表盘)

28. As a 登录用户, I want 8 个指标卡(当日消耗、Cost Estimate、缓存命中、连续活跃等), so that 打开即得全景。
29. As a 登录用户, I want 当日 hour×tool 堆叠时间线, so that 看清一天的工作节奏。
30. As a 登录用户, I want 近 30 天用量与成本柱图, so that 发现趋势和异常日。
31. As a 登录用户, I want 按 Tool / model / Token 构成 / Device 的分布图, so that 知道量花在哪。
32. As a 多设备用户, I want 看到每台 Device 的标签与最近同步时间, so that 发现哪台没在报。
33. As a 登录用户, I want 在设置页生成/吊销 User Token, so that 配置客户端和止血泄漏。

### 账号与成本

34. As a 新用户, I want 注册(昵称、密码、城市、头像)并登录, so that 上榜有身份。
35. As a 用户, I want 密码以 bcrypt 哈希存储、会话走 cookie, so that 基础安全达标。
36. As a 用户, I want Cost Estimate 按公开价格表折算并在价格表页公示单价与来源(official/estimated), so that 成本数字可解释。
37. As a 用户, I want 未知模型按模型家族兜底估算并标注 estimated, so that 新模型上线当天也有成本数字。
38. As a 用户, I want 价格表修正后历史成本自动重算(成本不落库), so that 口径修正即时全局生效。

### 防刷与运维

39. As a 维护者, I want 单设备单日超阈值自动打 Anomaly Flag 且当日不上榜, so that 明显异常刷不动榜。
40. As a 维护者, I want 被标记用户本人仍可见自己的数据, so that 误伤可自查。
41. As a 自托管者, I want 服务端是单个二进制 + SQLite 文件, so that 部署就是传一个文件。
42. As a 维护者, I want 价格表是启动时加载的独立 JSON, so that 不改代码就能调价。

## Implementation Decisions

- **仓库与上下文**:同仓库扩展。客户端命令与快照构建进 CLI 包与新的 report 包;服务端为新的入口与 server 内部包;排行榜域词汇表与 CLI 域分开维护(CONTEXT-MAP 双 context)。价格表 JSON 放服务端资源目录。
- **上报粒度**:Hourly Usage = (device, date, hour, tool, model) 上五类计数:input、output、cache read、cache write 5m、cache write 1h。cache write 拆分取自 Usage Entry 的 `cache_creation` 5m/1h 字段,无拆分对象时全部计入 5m。Tool 即 Agent Adapter 名。
- **当日过滤**:客户端按本地时区(复用现有 timezone 选项)过滤"今天"的 entries;只报当天,无回溯窗口。
- **Device ID**:首次上报前生成随机 UUID 持久化于用户配置目录的 device.json(含设备标签,默认 hostname)。文件丢失视为新设备,受 3 台上限约束。
- **上报协议**:`POST /v1/report`,Bearer User Token。payload 含 deviceId、deviceLabel、date、timezone、generatedAt、hours 数组。服务端语义:token 校验 → 设备查找/绑定(超 3 台拒绝)→ 事务内按 (device, date) 删除后批量插入 → 写审计行 → 返回接受结果与当日汇总。
- **latest-wins**:仅靠"当日全量快照 + 先删后插"实现,无版本号、无增量协议。
- **存储**(SQLite 单文件):users(昵称、密码哈希、城市、头像)、tokens(哈希)、devices(device_id 主键、属主、标签、时间戳)、hourly_usage(明细,主键 device+date+hour+tool+model,含 flagged 列)、reports(审计元数据,不存原始 payload)。
- **成本**:查询时现算,不落库。价格模块移植 CLI 域价格语义:input/output/cache read 单价、cache write 5m 按 input 价 1×、1h 按 2×、200K 边际分层(TieredCost)、OpenAI long-context 整档切换、fast 倍率;价格表 = LiteLLM 快照(种子取自 CLI 域内嵌数据)+ 模型家族前缀兜底估算,来源标 official/estimated。
- **Web**:SSR(html/template)+ ECharts(内嵌资源),筛选走 URL query 整页刷新;路由:/ Leaderboard、/me Dashboard(cookie 会话)、/pricing、/about(数据说明+上榜规则)、/login /register、设置页(User Token 生成/吊销)。无 Node 构建链,前端资源随二进制 embed。
- **服务端形态**:Go 单体单二进制,启动时加载价格表 JSON,监听一个端口同时服务 API 与页面。
- **防刷 v1**:单设备单日总 token 超阈值(初始 10 亿)置 flagged,当日聚合排除 flagged 行;审计表保留全部上报轨迹。设备验证/签名不做。
- **客户端配置优先级**:flag > 环境变量 > ccusage.json。
- **口径一致性约束**:客户端聚合必须复用 CLI 域的 adapter/dedup 代码路径,禁止平行实现,否则榜单与本地报表对不上。

## Testing Decisions

- **好测试的标准**:只测外部可观察行为——HTTP 响应、SSR 页面中的聚合数字、CLI 输出与退出码;不测内部函数、表结构或模块边界。
- **单一端到端接缝**(已与用户确认):测试内启动真实服务端实例(随机端口、临时 SQLite、测试价格表),对它执行真实 `ccusage report` 命令(指向 fixture 日志目录与测试 server 地址),断言:上报响应、再跑一次后的 Latest-wins 覆盖结果、Leaderboard/Dashboard 页面渲染出的 token 数与 Cost Estimate、设备上限拒绝、Anomaly Flag 排除。客户端、协议、存储、聚合、计价在同一个接缝下全覆盖。
- **fixture 复用**:沿用仓库现有模式——golden 测试通过环境变量指向 testdata 日志目录跑真实命令比输出;adapter loader 测试用同样的 fixture 思路。新增 fixture 覆盖:多 agent 混合日志、5m/1h 缓存拆分、dedup 场景、跨小时边界。
- **服务端页面断言**:对 SSR 页面断言关键数字(总额、名次、成本)与筛选参数组合的子集行为,不做像素级/DOM 全量比对。
- **先例**:internal/golden 的"跑真命令比输出"哲学直接延伸到端到端接缝。

## Out of Scope

- 查询用 JSON API(小程序/App 接入)——聚合 SQL 留复用,接口后补;
- 完整防刷风控(设备验证、请求签名、人工审核流);
- 回溯窗口(报今天以外的历史日期)与数据修正;
- 跨时区对齐(社区"今天"按各设备本地时区混合);
- 原始条目或会话内容上传统计(永久红线,ADR 0001);
- 原有 CLI 报表命令的行为变更;
- 头像文件上传(首期用预置头像/首字母);
- 多语言界面。

## Further Notes

- 已知取舍(用户确认):只报当天意味着当日末次上报之后的用量不计入当日,靠每小时 timer 缓解;
- 里程碑:M1 管道打通(report + /v1/report + SQLite)→ M2 Leaderboard → M3 Dashboard + 账号 → M4 全 agent + timer + 价格表页 + Anomaly Flag;
- 设计详情与 18 项决策记录见 `.scratch/token-leaderboard/design.md`;领域词汇见 `server/CONTEXT.md`;架构红线见 `docs/adr/0001-aggregate-only-reporting.md`。
