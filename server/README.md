# Token 排行榜服务端部署

与 CLI 同一个二进制:server 命令收在 `token-usage server` 命令组下(ADR 0011),SQLite 文件即可运行,前端资源(ECharts/样式/模板)全部内嵌,无外部文件依赖。

## 快速开始

只想本机看自己消耗的话不需要下面这套——`token-usage web` 一条命令即可(仅回环监听、免登录、进程内摄入,见 ADR 0012)。以下为社区部署流程:

```bash
# 构建单个二进制(交叉编译同理;客户端上报用的也是它)
go build -o token-usage ./cmd/token-usage

# 建库并创建第一个用户(打印一次性 report token)
token-usage server add-user --db leaderboard.db --name 你的名字 --city 北京

# 启动。默认只监听 127.0.0.1:8787;部署到服务器时显式指定对外地址
token-usage server serve --db leaderboard.db --addr 0.0.0.0:8787
```

浏览器打开 `http://<host>:8787/` 即是排行榜;`/pricing` 价格表、`/about` 数据说明、`/login` `/register` 账号。

## 命令与参数

| 命令 | 说明 |
|------|------|
| `server serve` | 启动服务;`--addr` 监听地址(默认 127.0.0.1:8787,即本机回环——远程部署需显式改),`--db` SQLite 路径,`--pricing` 价格表覆盖文件 |
| `server add-user` | 建用户并签发一次性 report token;`--name` 必填,`--city` `--password` 可选 |
| `server add-token` | 给已有用户追加一枚 report token;`--name` 必填,`--label` 可选 |
| `server dump-pricing` | 导出当前生效价格表为 model-prices.json(可编辑后用 `--pricing` 回喂) |

裸 `token-usage server`(不带子命令)只打印帮助,不会启动监听。

## 客户端接入

```bash
token-usage report --server http://<host>:8787 --token <token>   # 手动上报(重跑即覆盖当日)
token-usage report --install-timer                               # 安装每小时自动上报
token-usage report --since 2026-02-15                            # 一次性回溯:把该日期至今每天的数据在单个请求里上报
```

- 稳态语义不变:默认只报今天;`--since` 是一次性回溯(每 (设备, 日期) 仍是独立的 latest-wins 单元,可安全重跑);
- 回溯单请求上限:550 天 / 20 万行;无数据的天自动跳过,不会清空已有数据;
- 口径:claude 走与 `token-usage daily` 完全相同的 daily 管道(含 agent-progress 行与相同 dedup 决胜),榜单总量与本地报表可对账;榜单总量含缓存 token(codex 的 cached 计入 cache-read 类,本地 codex daily 的 totalTokens 不含缓存,差额即当日 cached)。

或在 token-usage 配置(`~/.config/token-usage/config.json`,见 ADR 0007)里配 `reportServer` / `reportToken`,或用环境变量 `CCUSAGE_REPORT_SERVER` / `CCUSAGE_REPORT_TOKEN`(优先级:flag > env > config)。

## 价格表

- 内嵌种子:LiteLLM 快照 + 内置表 + models.dev 兜底(与 CLI 域同源);
- 覆盖文件:`--pricing server/pricing/model-prices.json`,只需写要覆盖的模型,未列出的模型继续用种子价;支持部分字段覆盖;
- 未知模型按家族前缀兜底估算,页面标注 `estimated`;
- 成本查询时现算不落库:改价后重启服务,历史成本自动重算。

调价流程:`server dump-pricing` 导出 → 编辑 → 用 `--pricing` 指向后重启。

## 运维要点

- **备份**:停机复制 `leaderboard.db`(或用 `sqlite3 .backup`)即完成全量备份;上报数据可随时由客户端重传重建当日。
- **反代**:建议 nginx/caddy TLS 终结后反代到 `--addr`;健康检查可用 `GET /about`。
- **限速**:每 token 每小时最多 60 次上报、请求体上限 2MB,超限返回 429/413。
- **防刷**:单设备单日超 10 亿 token 自动打 Anomaly Flag(`hourly_usage.flagged`),当日不进榜单、本人 `/me` 可见;`reports` 表保留完整上报审计轨迹。
- **会话**:登录会话在内存中,重启服务需重新登录(report token 不受影响)。

## 目录

- `CONTEXT.md` — 排行榜域词汇表(与 CLI 域分开,见根目录 CONTEXT-MAP.md)
- `pricing/model-prices.json` — 价格表基线(dump-pricing 生成,可编辑)
- 服务端实现:`internal/server`(store/api/pricing/query/web),命令面在 `internal/cli/server.go`(挂 `token-usage server` 命令组)
- 端到端测试:`internal/e2e`(单二进制自扮演服务端与客户端全链路)
