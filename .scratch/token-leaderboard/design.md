# Token 排行榜系统设计

> 2026-08-15 · 基于 18 个设计决策(grilling 两轮)收敛而成。截图参照:ZCode Token 排行榜(个人仪表盘 / 排行榜 / 数据说明 / 价格表 / 上榜规则五页)。

## 0. 决策记录

| # | 决策 | 结论 |
|---|------|------|
| Q1 | 定位 | 先小圈子,按公开社区榜形态设计;账号/防作弊留接口不先做重 |
| Q2 | 范围 | 四块全做但做薄:客户端上报 + 服务端 + 排行榜页 + 个人仪表盘页 |
| Q3 | 客户端 | ccusage-go 仓库内新增 `report` 子命令,不独立仓库 |
| Q4 | 上报粒度 | (device, date, hour, tool, model) 五类 token 计数 |
| Q5 | 幂等 | 设备当日全量快照,latest-wins 覆盖 |
| Q6 | 身份 | 自建轻账号;网页登录生成 User Token,客户端凭 token 上报,设备自动绑定(上限 3 台) |
| Q7 | 成本 | 服务端统一折算,客户端不背价格表 |
| Q8 | 触发 | 手动 `ccusage report` + `--install-timer` 一键装 cron |
| Q9 | 服务端 | Go 单体单二进制(HTTP API + SSR 页面 + SQLite),前端资源 go:embed |
| Q10 | 存储 | 明细表 hourly_usage 按 (device, date) 先删后插;reports 表存上报元数据做审计 |
| Q11 | 回溯 | **只报当天**(用户指定;当日末次上报之后产生的用量不计入,建议装 timer 缓解) |
| Q12 | 时区 | 客户端本地时区切分 date/hour,复用现有 `--timezone` |
| Q13 | API | 仅 `POST /v1/report`;页面 SSR 直查库,查询 API 后补 |
| Q14 | 前端 | html/template + ECharts,筛选走 URL query 整页刷新,无 Node 构建链 |
| Q15 | 设备 ID | 首次生成随机 UUID 持久化于 `~/.config/ccusage/device.json`(附 hostname 标签) |
| Q16 | 防刷 | 仅异常标记:设备单日超阈值不上榜,reports 表留审计 |
| Q17 | agent 范围 | 全部 16 个 adapter,tool 字段即 agent 名 |
| Q18 | 仓库 | 同仓库双 context(CONTEXT-MAP.md 已建);词汇表 `server/CONTEXT.md` |

## 1. 总体架构

```
┌─────────────────────────┐        ┌──────────────────────────────────┐
│ 客户端 (ccusage-go)      │        │ 服务端 (cmd/server, 单二进制)      │
│                         │        │                                  │
│  16 个 agent adapter    │  HTTPS │  POST /v1/report  ← 接收/校验/落库 │
│  → dedup → 按本地时区    │ ─────► │  聚合 SQL → Leaderboard / Dashboard│
│    当日 (hour×tool×model)│  Bearer │  价格模块 → Cost Estimate          │
│    五计数聚合            │  token │  html/template + ECharts (embed)  │
│  device.json (UUID)     │        │  SQLite 单文件                     │
└─────────────────────────┘        └──────────────────────────────────┘
                                            ▲
                                   浏览器访问 SSR 页面
```

## 2. 客户端:`ccusage report`

**新增文件**:`internal/cli/report.go`(命令定义)、`internal/report/`(快照构建 + 上报客户端)。

**流程**:
1. 复用 `internal/adapter/all` 加载全部 16 个 agent 的 entries(与 CLI 报表同一口径,含 dedup);
2. 按客户端本地时区过滤出**今天**的 entries(Q12、Q11);
3. 按 `(tool, model, hour)` 分组,累加五类计数:`input / output / cache_read / cache_write_5m / cache_write_1h`(cache write 用 `CacheCreationTokenCount` 的 5m/1h 拆分);
4. 读取/生成 `~/.config/ccusage/device.json`(Q15);
5. `POST {server}/v1/report`,`Authorization: Bearer <user-token>`。

**配置**:server URL 与 user token 来自 `ccusage.json`(`reportServer`、`reportToken`)或环境变量 `CCUSAGE_REPORT_SERVER` / `CCUSAGE_REPORT_TOKEN`,命令行 flag 优先。

**`--install-timer`**:向用户 crontab 追加每小时执行 `ccusage report`(幂等:已存在则跳过)。缓解 Q11 的当日尾部丢量问题。

## 3. 上报协议

`POST /v1/report`(Bearer User Token)

```json
{
  "deviceId": "6f9619ff-8b86-d011-b42d-00cf4fc964ff",
  "deviceLabel": "wujunwei-wsl",
  "date": "2026-08-15",
  "timezone": "Asia/Shanghai",
  "generatedAt": "2026-08-15T21:03:00+08:00",
  "hours": [
    { "hour": 9,  "tool": "claude", "model": "claude-sonnet-4-5",
      "input": 120000, "output": 8000, "cacheRead": 900000,
      "cacheWrite5m": 50000, "cacheWrite1h": 10000 },
    { "hour": 9,  "tool": "codex",  "model": "gpt-5", "...": "..." }
  ]
}
```

**服务端语义**:
1. 校验 token → user;
2. 设备绑定:deviceId 已存在且属该 user → 更新 last_seen;不存在且该 user 设备 < 3 → 绑定;否则拒绝(409);
3. 事务:`DELETE FROM hourly_usage WHERE device_id=? AND date=?` → 批量 INSERT(latest-wins);
4. 写 `reports` 审计行;返回 `{ "accepted": true, "deviceCount": 2, "dayTokens": 12345678 }`。

**防护**:单请求体上限(如 2MB)、每小时 token 维度限速、字段类型严格校验。

## 4. 服务端数据模型(SQLite)

```sql
users(id, name, password_hash, city, avatar, created_at)          -- 轻账号
tokens(token_hash, user_id, label, created_at, last_used_at)      -- User Token 存哈希
devices(device_id PK, user_id, label, first_seen, last_seen)      -- ≤3/user(应用层约束)
reports(device_id, date, reported_at, entry_count, total_tokens)  -- 审计元数据,不存 payload
hourly_usage(                       -- 唯一明细表
  device_id, date, hour, tool, model,
  input, output, cache_read, cache_write_5m, cache_write_1h,
  flagged INTEGER DEFAULT 0,        -- Anomaly Flag
  PRIMARY KEY(device_id, date, hour, tool, model)
)
```

榜单查询全部走 `hourly_usage` 上的聚合 SQL(date + tool + model 有二级索引)。

## 5. 成本折算(服务端价格模块)

- 价格表:`server/pricing/model-prices.json`,启动时加载——LiteLLM 快照(从 `internal/core/pricingdata` 移植)+ 模型家族兜底估算(如 `claude-*` 未命中时按家族前缀取价),来源标 official / estimated(对应截图价格表页);
- 计费语义移植 `internal/core/cost.go`:input/output/cache read 单价、cache write 按 5m(1×input 价)与 1h(2×input 价)分开、`TieredCost` 200K 分层、OpenAI long-context 整档切换、`-fast` 倍率;
- 成本在查询时现算(总量 × 单价),不落库——价格表修正后历史自动重算(ADR 0001)。

## 6. Web 页面(SSR,html/template + ECharts)

| 路由 | 页面 | 要点 |
|------|------|------|
| `GET /` | Leaderboard | 筛选:tool / model / city / 时间范围(今天/昨天/前天/近3/近7/近30/全部)/ 缓存口径;全员累计消耗卡;排名列表(头像、模型徽标、token、Cost Estimate、设备数) |
| `GET /me` | Dashboard(需登录) | 8 指标卡;当日 hour×tool 堆叠时间线;近 30 天用量/成本柱图;按 tool/model/Token 构成/device 分布;设备列表与最近同步时间 |
| `GET /pricing` | 价格表 | 全模型单价 + 来源(official/estimated) |
| `GET /about` | 数据说明 + 上榜规则 | 对应截图第三、五页文案 |
| `GET /login` `GET /register` | 轻账号 | 密码 bcrypt;登录后可在设置页生成 User Token |

图表数据由模板内联 JSON 喂 ECharts;筛选全部 URL query 参数,整页刷新。

## 7. 防刷榜(v1)

- 阈值:单设备单日总 token > 10 亿 → `flagged=1`,该设备当日不进 Leaderboard,本人 Dashboard 仍可见;
- `reports` 表留完整上报轨迹(频次、条数、总量)供事后审计;
- 开放注册前再评估设备验证/签名(与 Q1 一致,本期不做)。

## 8. 目录结构(增量)

```
ccusage-go/
├── CONTEXT-MAP.md                  # 新:双 context 索引
├── CONTEXT.md                      # CLI 域词汇表(不动)
├── internal/cli/report.go          # report 子命令
├── internal/report/                # 快照构建、device.json、上报客户端
├── cmd/server/                     # 服务端入口
├── internal/server/                # api / store / aggregate / pricing / web
├── server/
│   ├── CONTEXT.md                  # 排行榜域词汇表(新)
│   └── pricing/model-prices.json   # 服务端价格表
└── docs/adr/0001-aggregate-only-reporting.md
```

## 9. 里程碑

- **M1 打通管道**:report 子命令(仅 claude 也行)+ /v1/report + SQLite 落库 + 命令行验证 latest-wins;
- **M2 排行榜页**:聚合 SQL + 价格模块 + `/` 页面与全部筛选;
- **M3 个人仪表盘**:账号登录 + User Token 签发 + `/me` 全部图表;
- **M4 收尾**:全部 16 agent、`--install-timer`、/pricing /about、Anomaly Flag。

## 10. 已知边界(用户确认过的取舍)

- **只报当天**(Q11):当日末次上报之后产生的用量不会计入当日;用户手动跑且当天不再跑,会丢当日尾部。缓解:装 timer 每小时跑;次日首跑只报次日。
- **跨时区口径**(Q12):各设备按本地时区上报,社区"今天"不严格对齐,可接受。
- **无查询 API**(Q13):页面 SSR 直查;未来小程序/App 需补 JSON API,聚合 SQL 可复用。
