# Token 排行榜

客户端读取本地 AI 编程 agent 日志,按日上报小时级聚合用量;服务端汇总成社区排行榜与个人仪表盘。本上下文与 token-usage CLI 共享"Usage Entry / Token Usage"等摄入术语(见根 `CONTEXT.md`),以下为排行榜域自有术语。

## Language

### 上报

**Report Snapshot**:
某设备某日的全部用量,以小时级聚合一次上报、整体生效;重传即整体覆盖。
_Avoid_: increment, delta, patch

**Device**:
一台安装了客户端的机器,由持久化的 Device ID 标识,每人最多 3 台;个人仪表盘按设备展示分布。
_Avoid_: machine, client, host

**Device ID**:
客户端首次上报前生成并持久化在本地配置中的随机标识;本地文件丢失后视为新设备。

**Latest-wins**:
同一设备同一 Report Date 以最近一次上报为准的覆盖语义;重新执行上报命令即修正当日数据。
_Avoid_: upsert, sync

**Report Date**:
按客户端本地时区切分的自然日,是上报与榜单"今天"的口径基础。
_Avoid_: UTC day

**Hourly Usage**:
上报的最小粒度:某设备某日某小时、某工具、某模型的五类 token 计数(input / output / cache read / cache write 5m / cache write 1h)。
_Avoid_: raw entry, log line

**User Token**:
客户端上报时持有的用户凭证,由用户在网页端生成后填入客户端配置。

### 展示

**Leaderboard**:
跨用户的用量排名页,支持按工具/模型/城市/时间范围/缓存口径筛选。
_Avoid_: ranking, board

**Dashboard**:
单用户个人统计页:指标卡、当日小时时间线、近 30 天趋势、按工具/模型/构成/设备的分布。
_Avoid_: profile, panel

**Tool**:
用量所属的编程 agent(claude、codex 等),榜单筛选维度之一;对应 CLI 域的 Agent Adapter,面向展示时称 Tool。

**Cache 口径**:
榜单汇总是否计入缓存 token 的开关:含缓存(全部四类)或仅新增(input+output)。

**Cost Estimate**:
服务端按公开价格表对 token 折算的成本估算,非账单金额。
_Avoid_: billing, spend

**Anomaly Flag**:
单设备单日用量超过阈值时打的标记;被标记设备当日不进入 Leaderboard,数据保留且本人可见。
