# 10 — 配置系统:`ccusage.json`

**What to build:** `ccusage.json` 的发现(`./.ccusage/ccusage.json` → `<claude-config-dirs>/ccusage.json`,`--config <path>` 覆盖)、sections(defaults、commands.{daily,monthly,weekly,blocks,statusline}、per-agent {claude,codex,pi,openclaw}、pricingOverrides、pi.stores)与合并优先级 **CLI > env > config > 默认**;config-schema 再生成;非法配置的报错文案对齐。

**Blocked by:** 03 — pricingOverrides 作用于定价体系。

**Status:** ready-for-agent

- [x] 发现序三形态(工作目录、claude 配置目录、`--config`)单测覆盖
- [x] 各 section 合并优先级 CLI > env > config > 默认的表驱动单测(注:参考实现已不再读取 CCUSAGE_OFFLINE 环境变量,offline 由 `OfflineEffective` 解析)
- [x] `pricingOverrides` 生效:config 单价覆盖实时/内嵌定价的 golden
- [ ] per-agent defaults(如 codex 专属默认值)作用于对应 Agent Adapter(config 侧已就绪:internal/config ApplyConfig/*AgentArgs;待 adapter 接线)
- [ ] config-schema 可由命令再生成,语义与参考 schema 一致(未做)
- [x] 非法 config(未知字段/类型错误)报错文案对齐(pi.stores 错误文案单测逐字对齐;未知字段/类型错误按参考实现静默忽略并有 golden 覆盖)

## Comments

2026-08-15(ticket 10 implementation):

- `internal/config` 落地:发现序(`./.ccusage/ccusage.json` → CLAUDE_CONFIG_DIR 逗号分隔各目录 → `$HOME/.config/claude`、`$HOME/.claude`;`--config path|=` 覆盖)、严格 JSON 对象解析(json.Number 保留整数字面量语义)、option maps 优先级(defaults < commands."claude daily" < commands.daily < commands."claude:daily" < claude.defaults < claude.commands.daily,已用参考二进制逐一验证)、pi.stores 校验与错误文案、pricingOverrides 整表 drop / 字段级合并语义。
- Apply 步骤:`config.ApplyConfig(command, agent, shared, changed, commandArgs...)`(lead 接线用,`changed` 收 flag 名)与 `config.ApplyToFlags(agent, report, flags, shared)`(经 pflag `flags.Set` 回灌,CLI 显式 flag 永不被覆盖)。
- 接线现状:`internal/cli/config_hook.go` + `claude.go` 中 registerSharedFlags 末尾一行 `WireCommandConfig(cmd, f.shared)`(ticket-10 唯一钩子调用点);`SharedArgs` 新增 `PricingOverrides`/`PIStores` 字段;claude adapter 两个定价加载点传入 `shared.PricingOverrides`。
- golden:17 个 `config-*` 用例(pricing override、invalid 整表丢弃、`--config=`、breakdown 表格、weekly startOfWeek、section 优先级、CLI 胜出、timezone、since/until、blocks、instances、未知字段/坏 JSON/缺失路径/claude 下 pi.stores 忽略、CLAUDE_CONFIG_DIR 发现)。golden harness 的 args 现支持 `$FIXTURE`/`$REPO` 展开(golden_test.go 与 scripts/golden.sh 同步修改)。
- 待 lead:顶层 all-agent 报告命令接 `ApplyConfig` 并在 `CommandUsesNamedPIStores` 时把 pi.stores 错误以 exit 2 输出(参考文案见 internal/config 单测);codex/pi/openclaw adapter 消费 per-agent defaults;config-schema 再生成命令。
