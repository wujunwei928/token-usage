# token-usage

[English](README.md) | [简体中文](README.zh-CN.md)

[ccusage](https://github.com/ryoppippi/ccusage)(Rust v20 系列)的 Go 移植,并作为独立工具 `token-usage` 重新命名:分析 Claude Code 及其他 AI 编程 agent 的本地日志,输出 token 用量与成本报表。

CLI 命令面、语义、退出码跟随参考实现;品牌字符串(命令名、版本行、帮助提示)刻意不同——这是一次有意的品牌独立,不是可无缝替换的别名([ADR 0008](docs/adr/0008-rebrand-to-token-usage.md))。内置的 `zcode` 适配器是超出上游的超集扩展([ADR 0006](docs/adr/0006-adapters-beyond-upstream.md)):它默认参与全 agent 报表,因此这些报表可能比参考实现多出行。配置发现使用自己的 `token-usage` 命名空间,不读上游的 Claude 配置目录([ADR 0007](docs/adr/0007-token-usage-config-namespace.md))。

在 CLI 之上还附带一个可选的 **Token 排行榜**:客户端把本地聚合用量上报给单二进制服务端,渲染社区排行榜和个人仪表盘,详见下方 [Token 排行榜](#token-排行榜)。

## 安装

```sh
go install github.com/wujunwei928/token-usage/cmd/token-usage@latest
```

或从源码构建:

```sh
go build -o token-usage ./cmd/token-usage
```

linux/darwin/windows(amd64/arm64)的发布二进制由 GoReleaser 构建(`.goreleaser.yml`)。

## 使用

```sh
token-usage                # 全 agent 日报表
token-usage claude daily       # 仅 Claude 的日报表
token-usage claude weekly --start-of-week monday
token-usage claude session --id <sessionId>
token-usage claude blocks      # 5 小时计费窗口
token-usage claude statusline  # Claude Code 状态栏 hook(读 stdin)
token-usage codex daily        # Codex 报表
token-usage zcode daily        # ZCode 报表(读取 ~/.zcode/cli 的用量分析库)
token-usage daily --sections daily,weekly,monthly,session --json
```

运行 `token-usage --help` 或任意子命令加 `--help` 查看完整参数。

### ZCode

`zcode` 适配器是超出上游 ccusage 的扩展([ADR 0005](docs/adr/0005-zcode-adapter-sqlite-source.md)、[ADR 0006](docs/adr/0006-adapters-beyond-upstream.md))。它不扫描 JSONL,而是只读 ZCode 的本地用量分析库(`~/.zcode/cli/db/db.sqlite`;可用 `ZCODE_DATA_DIR` 覆盖),按「每次模型调用尝试」计数——重试、失败调用、辅助调用(如会话标题)都计入——子代理会话归并到父会话。早于 `model_usage` 表的历史会话用同库的逐消息 token 回退补齐。GLM 系模型通常不在定价表中:补价之前成本显示 `$0.00` 并给出 missing pricing 警告,可在 token-usage 配置的 `pricingOverrides` 中补录。

### 配置

`ccusage.json` 风格的配置放在 token-usage 自己的 `token-usage` 命名空间里——刻意不读上游 ccusage 的配置文件(Claude 配置目录),两个工具永不共享配置([ADR 0007](docs/adr/0007-token-usage-config-namespace.md))。发现顺序:`./.token-usage/config.json` → `~/.config/token-usage/config.json` → `~/.token-usage/config.json`;`TOKEN_USAGE_CONFIG_DIR` 覆盖全局查找,`--config` 指向任意文件。文件格式(`defaults`/`commands` 分节、`pricingOverrides` 等)与上游一致。

## Token 排行榜

构建在同一套 agent 适配器之上的端到端用量排行榜:客户端把每天用量聚合成「小时 × 工具 × 模型」的 token 计数并上报 Report Snapshot;服务端存入 SQLite 并渲染网页。原始日志条目永不离开本机(见 `docs/adr/0001-aggregate-only-reporting.md`)。

```sh
# 一键演示:构建两个二进制、灌入 5 用户 × 30 天数据、在 :8787 起服务
scripts/demo.sh [端口]

# 服务端(单二进制 + SQLite,网页资源全部内嵌)
go build -o token-usage-server ./cmd/server
./token-usage-server add-user -db leaderboard.db --name alice --city 北京
./token-usage-server serve -db leaderboard.db --addr 0.0.0.0:8787

# 客户端:上报今天(重跑即覆盖当日,latest-wins)
token-usage report --server http://<host>:8787 --token <token>
token-usage report --since 2026-02-15   # 一次性回溯,单请求多日
token-usage report --install-timer      # 安装每小时自动上报的 crontab
token-usage report --dry-run            # 只打印快照不上报
```

服务端地址与 token 按优先级解析:命令行 flag > 环境变量 `TOKEN_USAGE_REPORT_SERVER`/`TOKEN_USAGE_REPORT_TOKEN`(旧名 `CCUSAGE_REPORT_*` 仍生效)> token-usage 配置(`reportServer`/`reportToken`)。

网页:`/` 排行榜(工具/模型/城市/时间范围/缓存口径筛选)、`/me` 个人仪表盘(指标卡、当日小时×工具时间线、近 30 天趋势、多维度分布、设备列表)、`/pricing` 价格表(453 个模型,official/estimated 来源标注)、`/about` 数据说明与上榜规则,以及注册/登录/设置页(一次性展示的 User Token 签发与吊销)。防护:每用户最多 3 台设备、请求体上限 2MB、每 token 每小时 60 次上报、单设备单日超 10 亿 token 打异常标记。

部署、价格表覆盖与运维说明见 [`server/README.md`](server/README.md);领域词汇表见 [`server/CONTEXT.md`](server/CONTEXT.md)。

## 一致性验证

- `scripts/golden.sh [参考二进制] [用例前缀]` 从参考实现再生成字节级 golden 文件(品牌独立后需重放 ccusage→token-usage 字符串调整,见 [ADR 0008](docs/adr/0008-rebrand-to-token-usage.md);入库的 golden 才是规格)。
- `go test ./...` 将 Go 二进制的输出与 golden 逐字节比对(无需 Rust 工具链)。
- `scripts/compare.sh` 用本机真实数据对已安装的 `ccusage` 做实时差分。

## 开发

- 目录结构对应参考实现的 Rust crate:`internal/core`(类型/成本/价格/聚合)、`internal/terminal`(表格渲染)、`internal/adapter/<agent>`(每个 agent 一个包)、`internal/cli`(cobra 命令树)。排行榜新增 `internal/report`(快照构建与上报客户端)和 `cmd/server` + `internal/server`(接收、存储、价格、SSR 网页)。见 `docs/rewrite-plan.md` 与 `docs/adr/`。
- `internal/e2e` 运行全链路测试:真实服务端二进制 + 真实 `token-usage report` 命令跑 fixture 日志。
- 价格快照通过 `go:embed` 内嵌;用 `scripts/update-pricing.sh` 刷新。
