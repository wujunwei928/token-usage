# ccusage-go

[English](README.md) | [简体中文](README.zh-CN.md)

[ccusage](https://github.com/ryoppippi/ccusage) 的 Go 实现,跟踪 Rust v20 系列(`20.0.19`)。它分析 Claude Code 及其他 AI 编程 agent 的本地日志,输出 token 用量与成本报表。

CLI 命令面、输出字节、退出码与参考实现完全一致——用户脚本和 shell 别名可以在两个实现之间无缝切换。

在 CLI 之上还附带一个可选的 **Token 排行榜**:客户端把本地聚合用量上报给单二进制服务端,渲染社区排行榜和个人仪表盘,详见下方 [Token 排行榜](#token-排行榜)。

## 安装

```sh
go install github.com/wujunwei/ccusage-go/cmd/ccusage@latest
```

或从源码构建:

```sh
go build -o ccusage ./cmd/ccusage
```

linux/darwin/windows(amd64/arm64)的发布二进制由 GoReleaser 构建(`.goreleaser.yml`)。

## 使用

```sh
ccusage                    # 全 agent 日报表
ccusage claude daily       # 仅 Claude 的日报表
ccusage claude weekly --start-of-week monday
ccusage claude session --id <sessionId>
ccusage claude blocks      # 5 小时计费窗口
ccusage claude statusline  # Claude Code 状态栏 hook(读 stdin)
ccusage codex daily        # Codex 报表
ccusage daily --sections daily,weekly,monthly,session --json
```

运行 `ccusage --help` 或任意子命令加 `--help` 查看完整参数。

## Token 排行榜

构建在同一套 agent 适配器之上的端到端用量排行榜:客户端把每天用量聚合成「小时 × 工具 × 模型」的 token 计数并上报 Report Snapshot;服务端存入 SQLite 并渲染网页。原始日志条目永不离开本机(见 `docs/adr/0001-aggregate-only-reporting.md`)。

```sh
# 一键演示:构建两个二进制、灌入 5 用户 × 30 天数据、在 :8787 起服务
scripts/demo.sh [端口]

# 服务端(单二进制 + SQLite,网页资源全部内嵌)
go build -o lbserver ./cmd/server
./lbserver add-user -db leaderboard.db --name alice --city 北京
./lbserver serve -db leaderboard.db -addr 0.0.0.0:8787

# 客户端:上报今天(重跑即覆盖当日,latest-wins)
ccusage report --server http://<host>:8787 --token <token>
ccusage report --since 2026-02-15   # 一次性回溯,单请求多日
ccusage report --install-timer      # 安装每小时自动上报的 crontab
ccusage report --dry-run            # 只打印快照不上报
```

服务端地址与 token 按优先级解析:命令行 flag > 环境变量 `CCUSAGE_REPORT_SERVER`/`CCUSAGE_REPORT_TOKEN` > `ccusage.json`(`reportServer`/`reportToken`)。

网页:`/` 排行榜(工具/模型/城市/时间范围/缓存口径筛选)、`/me` 个人仪表盘(指标卡、当日小时×工具时间线、近 30 天趋势、多维度分布、设备列表)、`/pricing` 价格表(453 个模型,official/estimated 来源标注)、`/about` 数据说明与上榜规则,以及注册/登录/设置页(一次性展示的 User Token 签发与吊销)。防护:每用户最多 3 台设备、请求体上限 2MB、每 token 每小时 60 次上报、单设备单日超 10 亿 token 打异常标记。

部署、价格表覆盖与运维说明见 [`server/README.md`](server/README.md);领域词汇表见 [`server/CONTEXT.md`](server/CONTEXT.md)。

## 一致性验证

- `scripts/golden.sh [ref-binary] [case-prefix]` 从参考实现再生成字节级 golden 文件。
- `go test ./...` 将 Go 二进制的输出与 golden 逐字节比对(无需 Rust 工具链)。
- `scripts/compare.sh` 用本机真实数据对已安装的 `ccusage` 做实时差分。

## 开发

- 目录结构对应参考实现的 Rust crate:`internal/core`(类型/成本/价格/聚合)、`internal/terminal`(表格渲染)、`internal/adapter/<agent>`(每个 agent 一个包)、`internal/cli`(cobra 命令树)。排行榜新增 `internal/report`(快照构建与上报客户端)和 `cmd/server` + `internal/server`(接收、存储、价格、SSR 网页)。见 `docs/rewrite-plan.md` 与 `docs/adr/`。
- `internal/e2e` 运行全链路测试:真实服务端二进制 + 真实 `ccusage report` 命令跑 fixture 日志。
- 价格快照通过 `go:embed` 内嵌;用 `scripts/update-pricing.sh` 刷新。
