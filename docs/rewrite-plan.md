# ccusage-go 重写方案

以 `/code/ai/ccusage/ccusage`(Rust v20.0.19)为基准的完整对等 Go 重写。CLI 框架使用 cobra。

对等承诺(见 ADR-0001~0004):

| 维度 | 承诺 |
|---|---|
| 功能 | 全部 6 命令 + 16 agent 适配器 + config + 定价体系 |
| CLI | 行为级对等:旗标、别名、默认值、退出码、报错文案、legacy 兼容层;仅 `--help` 布局用 cobra 默认 |
| 输出 | 字节级对等:表格(边框/对齐/ANSI/宽度截断/紧凑切换)与 JSON(形状与字段顺序) |
| 验证 | Rust 二进制生成 golden file 提交入仓,CI 纯 Go 字节 diff |

## 1. 仓库结构(单 module 镜像 crate,ADR-0004)

```
ccusage-go/
├── cmd/ccusage/            # ≈ crates/ccusage:main、命令实现、http fetcher、blocks
├── internal/cli/           # ≈ ccusage-cli + ccusage-cli-parser:cobra 命令树、参数类型、legacy 兼容层
├── internal/core/          # ≈ ccusage-core:types、cost、pricing、summary、output、model alias
├── internal/config/        # ≈ ccusage-config:ccusage.json 发现/合并/schema 生成
├── internal/terminal/      # ≈ ccusage-terminal:SimpleTable、显示宽度、ANSI
├── internal/adapter/       # ≈ rust/adapters
│   ├── common/             # jsonl 解析助手、按大小均衡的并行读取、共享报表打印
│   ├── all/                # ≈ ccusage-adapter-all:全 agent 统一报表
│   ├── claude/  codex/  opencode/  amp/  …(共 16 个)
├── internal/testsupport/   # ≈ ccusage-test-support:fs fixture、env guard
├── testdata/
│   ├── fixtures/           # 从 apps/ccusage/test/fixtures 迁移的共享输入
│   └── golden/             # Rust 二进制生成的字节级期望输出
├── scripts/golden.sh       # 用本地 Rust 构建再生 golden(仅开发期需要)
└── .goreleaser.yml
```

依赖(最小策略):`cobra`、`golang.org/x/term` 必选;`golang.org/x/text` 仅当决定不自研宽度表时引入(见开放问题);其余全用标准库,含 `time/tzdata` 内嵌时区(静态二进制跨机一致)、`net/http`(仅 cmd 层注入 core,保持 core 无网络)、`encoding/json`。

## 2. CLI 层设计(internal/cli + cmd/ccusage)

cobra 之上达成行为级对等的手段:

- **legacy 归一化**:root 的 `PersistentPreRunE` 之前,用参考实现的 `normalize_legacy_agent_command_args` 等价逻辑重写 `os.Args`(`codex:daily` → `codex daily`),cobra 感知的是归一化后的参数。
- **上下文相关的 `-a`**:只在 `blocks` 命令注册;其他命令命中 `-a` 时输出了与参考一致的"已移除 --agent"迁移报错(自定义 `FlagErrorFunc` + `UnknownFlags` 扫描)。
- **已移除旗标报错**:`--agent`、`--daily`/`--weekly` 等报表开关被显式使用时,给出带迁移指引的报错文案(逐字对照 `ccusage-cli-parser/src/parser.rs` 的错误快照)。
- **参数类型**:移植 `ccusage-cli/src/types.rs` 为 Go 结构(`SharedArgs`、各命令专属参数、`CostMode`/`Order`/`VisualBurnRate` 等枚举),cobra 绑定只做解析,语义校验(互斥:`--last` vs `--since/--until`/`--sections`)放在命令实现层,报错文案对齐。

命令树:`daily`(默认,裸 `ccusage` = all-agent daily)、`monthly`、`weekly`、`session`、`blocks`、`statusline` + 16 个 agent 子命令(各自支持的报表子命令集合按 `agent_report_supported()` 精确移植)。

## 3. 数据摄入(internal/adapter)

- **发现**(`claude/paths.go` ≈ `paths.rs`):`CLAUDE_CONFIG_DIR`(逗号分隔,可为 config 根或 `projects/` 本身)→ `$XDG_CONFIG_HOME/claude/projects` + `~/.claude/projects` 双扫去重;递归收 `**/*.jsonl` 按路径排序;project/session 名提取规则(`chat.jsonl` 取父目录、`subagents/` 取祖父目录)逐条移植。
- **加载**(`common/`):按文件大小把文件均衡分给 `GOMAXPROCS` 个 goroutine(errgroup),结果按原顺序合并;`--single-thread` 退化为单协程。逐行:`bytes.Contains(line, []byte("\"usage\":{"))` 预过滤 → 非空字段扫描(null 拒绝清单照抄)→ `json.Unmarshal` 到窄结构体 → 时间戳解析 → `date = ts.In(tz)` 格式化 → 附加 advisor iteration 子条目。
- **去重**:键 `(message.id, requestId)` + `(message.id, "")` sidechain 容错桶;同键保留优先级:非 sidechain > token 总量大 > 带 `speed`。移植 `push_deduped_entry` 的完整比较序。
- **适配器接口**:

```go
type Adapter interface {
    Name() string
    Discover(opts LoadOptions) ([]UsageFile, error)
    Load(ctx context.Context, files []UsageFile, opts LoadOptions) ([]LoadedEntry, error)
    SupportedReports() []ReportKind
}
```

非 JSONL 源(amp 的 JSON threads、copilot 的 OTel 导出)在各自 adapter 内转换为统一 `LoadedEntry`。codex 的 fork 重放去重(`replay.rs`)随 codex 适配器移植。

## 4. 定价与成本(internal/core)

- **快照嵌入**:`internal/core/pricing/embedded/` 下提交三份紧凑化数据(LiteLLM `model_prices_and_context_window.json`、models.dev `api.json`、`fast-multiplier-overrides.json`),`go:embed` 打入二进制;`make update-pricing` 从同 URL 再生并提交(替代 Rust 的 Nix pin)。
- **运行时拉取**:`net/http` 实现仅存在于 `cmd/ccusage`,以 `FetchJSON func(url string) ([]byte, error)` 接口注入 core(10s 超时、64MB 上限);失败仅告警回退嵌入快照;models.dev 失败后 60s 内不重试;`--offline`/`CCUSAGE_OFFLINE` 跳过。
- **成本三模式**:`auto`/`calculate`/`display` 语义与 `cost.rs` 一致;分层计价(LiteLLM >200K 为边际分层 `tiered_cost`;OpenAI 式 `long_context_threshold` 为整单切换)、1h cache-create 按 2× input、`speed:"fast"` 乘 `fast_multiplier` 并派生 `-fast` 模型后缀——全部移植并在单测中用参考实现的测试值对拍。
- **模型解析链**:精确 → 别名表(`CCUSAGE_MODEL_ALIASES`,JSON 或 `a=b` 格式)→ 最长候选的模糊后缀/日期后缀匹配 → 实时 models.dev → 内嵌 models.dev;缺价模型记账并告警。
- **格式化对齐**:成本显示的小数位/分组格式需与 Rust 输出逐字一致,作为 M2 的一项专项 golden 项。

## 5. 输出(internal/terminal + internal/core/output)

- **SimpleTable 移植**:制表符边框(┌┬┐…)、逐列左右对齐、多行单元格、表头蓝/ACTIVE 绿/百分比红、<100 列(usage)/<120 列(blocks)切紧凑布局、日期压缩;颜色码手写(`\x1b[…m`)保证字节对等,`--color`/`NO_COLOR`/`FORCE_COLOR` 优先级照抄。
- **显示宽度**:需要 East-Asian Width + emoji 感知;方案见开放问题。
- **JSON**:结构体字段序即输出序(`encoding/json` 保序);`--no-cost` 剔除成本字段;`--jq` 通过 `exec.Command("jq", filter)` 管道执行并透传退出码(jq 缺失时报错文案对齐)。

## 6. blocks 与 statusline(cmd/ccusage)

- **blocks**:5 小时窗切割算法(`floor_to_hour` 起算、`since_start > duration || since_last > duration` 切割、gap 灰块、active 判定)、burn rate、projection、token-limit ≥80% 告警、`--active`/`--recent`(3 天)/`--token-limit`/`--session-length`,全参数照抄 `blocks.rs`。
- **statusline**:stdin hook JSON 解析(session/model/cost/context_window/effort)、会话成本 + 当日成本 + active block 指示器(🟢/⚠️/🚨 阈值 2000/5000 非 cache tok/min)、上下文占用 50/80 阈值着色;`${TMPDIR}/ccusage-semaphore/<session>.lock` 缓存(transcript mtime + `--refresh-interval` 键控、活动进程信号量)整套移植;usage-limit reset 提示解析。

## 7. 配置(internal/config)

`./.ccusage/ccusage.json` → `<claude-config-dirs>/ccusage.json` 发现序、`--config` 覆盖;sections(`defaults`、`commands.*`、per-agent、`pricingOverrides`、`pi.stores`)与合并优先级 **CLI > env > config > 默认** 照抄 `ccusage-config`;提供 schema 生成命令保持 `config-schema.json` 可再发布。

## 8. 测试与对等验证

- **golden(核心机制)**:`scripts/golden.sh` 在固定环境(`NO_COLOR=1`、`COLUMNS` 固定、`TZ` 固定、fixture 根)下跑本地 Rust 二进制,产出 `testdata/golden/**`;Go 侧 `TestGolden/*` 以相同环境跑 Go 二进制做字节 diff。命令 × 输出模式(表格/JSON)× 关键旗标组合全覆盖。CI 不需要 Rust 工具链。
- **单测移植重点**(对拍参考实现测试值):dedup 替换序、null 字段拒绝、分层计价、`-fast` 后缀、advisor iteration 拆分、sidechain 重放、时区格式化、block 切割边界、statusline 缓存键。
- **fixture 基建**:`internal/testsupport` 提供等价 `fs_fixture` 的临时目录构造器与 `CLAUDE_CONFIG_DIR` 等 env guard(`t.Setenv`)。
- Rust `main.rs` 内嵌测试清单作为移植 conformance checklist 维护。

## 9. 里程碑(claude 纵深优先,每个里程碑以 golden 通过为验收)

| # | 内容 | 验收 |
|---|---|---|
| M0 | 脚手架:module、cobra 命令树、全旗标与 legacy 层、`--help` 快照测试、golden 基建 + 差异脚本 | 参数解析层与 Rust 报错快照一致 |
| M1 | claude 摄入:paths、并行加载、行管线、dedup | 摄入单测(对拍参考测试值)全绿 |
| M2 | 定价 + 成本 + SimpleTable 渲染器:嵌入快照、fetch 注入、三模式、alias、格式化对齐 | 成本/渲染 golden 通过 |
| M3 | 四报表 daily/weekly/monthly/session:`--breakdown/--instances/--project/--since/--until/--last/--order`、JSON 输出 | 全报表 golden 通过 |
| M4 | blocks + statusline(信号量缓存) | blocks/statusline golden + 单测 |
| M5 | config(`ccusage.json` 全 schema + 优先级)、`--jq`、all-report 骨架(仅 claude) | config 合并单测 + golden |
| M6 | codex + opencode 适配器(含 codex fork 重放去重、`--speed`) | 两 agent 全报表 golden |
| M7 | 其余 13 适配器 + all-report 完整化(`--sections`、`--by-agent`、并行加载) | all-report golden;16 agent 齐 |
| M8 | GoReleaser(多平台 + 版本注入)、性能核对(并行/预过滤/分配)、README | 发布产物可用 |

## 10. 开放问题(实现中决策,不阻塞方案)

1. **显示宽度实现**:内嵌生成版 Unicode 宽度表(零依赖,与 Rust `unicode-width` 对齐到同一 Unicode 版本)vs 引入 `golang.org/x/text`。倾向前者,以完全控制字节对等;M2 前定。
2. **golden 环境的 TTY 探测**:Rust 二进制在非 TTY 下的默认列宽/颜色行为需在 `scripts/golden.sh` 里显式钉死(固定 `COLUMNS` + `NO_COLOR`/`FORCE_COLOR`),首次生成前需验证。
3. **浮点格式化**:Rust 与 Go 的浮点转字符串路径不同,成本列格式化须以"格式化后字符串"对齐,禁止依赖默认 `%v`。
4. **嵌入定价快照的更新节奏**:跟随上游 LiteLLM 更新频繁,建议 M8 前只随里程碑手动更新,发布前一次性刷新。
5. **npm 启动器**:后置里程碑(M8 之后可选),架构上 Go 单二进制天然适配现有 launcher 的 spawn 模型。
