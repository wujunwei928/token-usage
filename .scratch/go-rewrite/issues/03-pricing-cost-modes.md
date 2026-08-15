# 03 — 定价体系 + 成本三模式

**What to build:** 无 costUSD 的 Usage Entry 按 Pricing 正确计价。交付:`--mode auto|calculate|display` 三种 Cost Mode;内嵌定价快照(LiteLLM / models.dev / fast-multiplier-overrides 三份,go:embed);运行时拉取(经注入的 FetchJSON 回调,10s 超时、64MB 上限,失败仅告警回退内嵌);模型解析链;缺价告警。计价规则含:分层 >200K 边际计价、OpenAI 式 long-context 整单切换、cache-create 默认 1.25× input / cache-read 0.1× input、1h cache-create 2× input、`speed:"fast"` 乘数与 `-fast` 模型后缀。

**Blocked by:** 02 — tracer bullet。

**Status:** ready-for-human

- [x] 三份定价快照内嵌随二进制分发,离线模式下计算可用
- [x] core 纯函数表驱动单测(辅测试接缝):分层 >200K、long-context 切换、cache 默认倍率、1h 2×、fast 乘数/`-fast` 后缀——测试值移植自参考实现测试
- [x] `--mode` 三模式各有 golden(同一 fixture 三种输出字节一致)
- [x] 模型解析链单测:精确 → `CCUSAGE_MODEL_ALIASES`(JSON 与 `a=b,c=d` 两格式)→ 最长模糊后缀 → 实时 models.dev → 内嵌
- [x] 运行时拉取走注入回调,失败告警回退内嵌;`--offline` / `CCUSAGE_OFFLINE` 跳过网络;models.dev 失败 60 秒内不重试
- [x] 缺价模型被记账、输出与参考一致的告警,报表行照常产出

## Comments

- 实现于 `internal/core/pricing.go`(PricingMap / 解析链 / 内嵌表 / 60s models.dev 节流 / SetJSONFetcher 注入)、`internal/core/cost.go`、`internal/core/aliases.go`、`internal/cli/http.go`(net/http 10s+64MB,由 `cmd/ccusage/main.go` 注入)、`internal/adapter/claude/loader.go` 的 `loadPricing`。
- 内嵌 LiteLLM 快照取自参考仓库 flake.lock 锁定的 rev(`f99d0a4b…`),按 build.rs 同样规则压缩;另两份为参考源文件的原样拷贝。
- 全部 pricing-* golden 用 `-O/--offline` 生成以保证确定性(参考二进制默认实时拉取)。`pricing-daily-offline` 最初用 `CCUSAGE_OFFLINE=1`,但参考 Rust 二进制 v20.0.19 实测忽略该环境变量(其缺价告警输出在线文案;仓库无任何读取该变量的代码,仅 docs 描述了 TS 旧版行为),该 case 改用 `--offline` 长旗标。Go 侧 `OfflineEffective()` 仍支持该环境变量(01-02 已定行为),属与参考的已知偏差。
