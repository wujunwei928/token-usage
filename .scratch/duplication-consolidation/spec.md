# Spec: 重复逻辑收敛与工厂闭包注入补全

Status: needs-triage

> 来源:2026-08-16 code-review 对未提交统一重构改动的审查(Standards 轴坏味道:Duplicated Code ×3、Repeated Switches;Spec 轴:工厂闭包注入仅 registry 路径成立的部分实现)。前置:`.scratch/unified-adapter-interface` 与 `.scratch/agent-command-framework` 两 spec 的改动。含两项待定决策(Q1/Q2),定案前不宜整批开工。

## Problem Statement

统一重构消除了大部分样板,但审查发现四处残留的重复/平行实现,与一处未兑现的原定决策:

1. **定价加载重复**:`internal/cli/agenttree.go:385-391` `agentPricing` 与 `internal/adapter/common/pricing.go:12-18` `LoadPricingRawOffline` 逐行相同(refreshLog 门 + `core.LoadWithOverrides`);同款 LOG_LEVEL 门第三次出现在 `LoadPricingDisplayGated`(pricing.go:26-29)。
2. **日期过滤三胞胎静默分歧**:`internal/adapter/all/spec_claude.go:72` `filterSessionSummaries`、`:89` `filterDailySummariesByDate` 与 `internal/adapter/common/pipeline.go:100` `filterRowsByLastActivity` 形状相同、仅取值字段不同(LastActivity vs Date);但 claude 两份先 `strings.ReplaceAll(date, "-", "")` 再比较,共享管线那份用原始串——`core.DateWithinRange` 对两种输入是否等价未经验证,三个相似实现随时漂移。
3. **kind↔token 平行映射**:`internal/cli/agenttree.go:326` `kindForToken` 手写了 `core.ReportKind.String()`(`internal/core/reportkind.go:17-28`)的逆映射;两者必须同步演进,新增 kind 时漏改即静默失配。
4. **工厂闭包注入未完成**(原 spec user story 13 已定案):unified-adapter-interface spec 定"agent 专属 flag 经工厂闭包在 adapter 构造时注入,不进共享请求结构"。现状 CLI 仍直接调包级 `LoadEntries` 并自组 options——`internal/cli/agent_pi.go:27-34`(`--pi-path` → `CustomPath` + `agentPricing` 内联)、`internal/cli/agent_openclaw.go:24-33`;`internal/adapter/pi/adapter.go:8-10` 注释自认"自定义路径在 registry 加载时保持 nil"。接口保持窄(目标达成),但带 flag 运行的入口仍是 CLI 闭包而非 adapter 工厂,两条路径的分歧行无 golden 钉住。

另有一项低优先级判断题:约 14 个新 `adapter.go` 重复同一 struct + `Agent()`/`HasData()`/`LoadEntries`/`init` 形状(goose 与 kilo 除名字外相同);按包 init 注册是地道 Go,默认不做(见 Out of Scope)。

## Solution

- **定价**:CLI 删 `agentPricing` 改调 `common.LoadPricingRawOffline`;LOG_LEVEL 门提为 common 内命名助手,三处共用。
- **过滤**:Q1 定案后收敛为 common 一份按字段参数化的实现;claude 独立 daily 管线的对外语义以 golden 为红线。
- **映射**:`core.ReportKind` 增 `Parse(token string) (ReportKind, bool)`,`kindForToken` 删除,CLI 改调。
- **工厂闭包**:pi/openclaw 的 CLI 路径改为经 adapter 工厂闭包注入 flag 值(`CustomPath` 等),`LoadEntries(LoadRequest{Shared})` 统一经 adapter 调用;范围以 Q2 定案为准。

## Decisions(待定)

| # | 决策 | 待定内容 | 倾向 |
|---|------|----------|------|
| Q1 | `-` 剥离统一方向 | 统一到"剥离后比较"还是"原始串比较",取决于 `core.DateWithinRange` 对 Since/Until 格式的实际预期;需先事实核查两种输入是否等价 | 事实核查后取与 Since/Until 归一化一致的一侧;若不等价,claude 现行为优先(有字节级 golden 钉住) |
| Q2 | 工厂闭包补全范围 | 仅 pi/openclaw 两处 CLI 直调改走工厂闭包,或顺带把 CLI 侧 `agentPricing` 内联定价也收进工厂(与候选 3"计价收口"的边界) | 最小范围:只改 flag 注入路径,计价加载语义不动(统一收口归候选 3) |

## User Stories

1. 作为维护者,我想让定价加载逻辑只有一份,这样修 LOG_LEVEL 门控不用改三处
2. 作为维护者,我想让日期过滤只有一份实现,这样新的分歧行为不会再静默固化
3. 作为维护者,我想让 kind↔token 映射在 core 单点维护,这样新增 kind 不会漏改逆映射
4. 作为 pi/openclaw 用户,我想让 `--pi-path`/`--open-claw-path` 经工厂闭包注入(兑现原 spec user story 13),这样 registry 与 CLI 两条路径同一入口语义

## 验收清单

- [ ] Q1/Q2 定案(或获授权按事实自定)
- [ ] `agentPricing` 删除,CLI 调 `LoadPricingRawOffline`;LOG_LEVEL 门单点
- [ ] 日期过滤收敛为一份实现;142 golden 字节不变为红线(若有 golden 牵动,回到 Q1 重新定案)
- [ ] `kindForToken` 删除,`core.ReportKind.Parse` 接管
- [ ] pi/openclaw 的 CLI 路径经 adapter 工厂闭包注入 flag
- [ ] `go build ./...` / `go vet ./...` / 全量测试绿;142 golden 字节不变

## Out of Scope

- ~14 个 adapter.go 的 struct 形状收拢(地道 Go per-package init,判断题,默认不做)
- `periodLabel` 文案表(ADR 0010 定标题文案属 CLI 层展示配置,非平行映射)
- 计价调用纪律统一(mode 硬编码、MissingPricing 配对、加载语义收口)——候选 3
- Detected 语义统一(HasData ∀ agent)——原 spec 已列为另行评审

## 建议的 issue 拆分(留待 /to-tickets)

1. 定价:删 `agentPricing` + LOG_LEVEL 门提为 common 助手
2. 过滤:Q1 事实核查与定案 + 三胞胎收敛为一份
3. 映射:`core.ReportKind.Parse` + 删 `kindForToken`
4. 工厂闭包:pi/openclaw CLI 路径改经 adapter 构造(Q2 范围)

## 第二轮审查新增(2026-08-16 meta-review 后复核,均为判断题,待与 Q1/Q2 一并定案)

1. **Agent 名单五处平行维护**(Shotgun Surgery 实证):`common/registry.go` `rosterOrder`、`cli/normalize.go` `agentNames`、`cli/errors.go` `agentDisplayName`、`all/report.go` `AgentLabel`、`adapter/register/register.go` 空白导入——grok 删除时 `AgentLabel` 漏网即症状。另 `agentNames` 与 `rosterOrder` 顺序不一致(openclaw/qwen 错位;两表仅成员语义,无排序契约,但收敛时应一并定序)。
2. **渲染旗标 Data Clump**:`sessionMeta`/`totalsNullEmpty` 自 `agentCommandSpec` → `runAgentReport` → `printAgentReport`(6 参)→ `common.AgentReportJSON` 结伴穿层,可捆成一个渲染旗标小结构。
3. **`ReportKind` 平行小 switch**:`agenttree.go` `lastPeriodUnit`(:372)与 `reportkind.go` 方法集之外的又一个 kind→X 级联(`periodLabel` 已由 ADR 0010 定性为 CLI 展示配置,不属此列)。
4. 亚微观察(记录不处理):`cli/errors.go` `agentDisplayName` 无 `zcode` 键,回退分支返回 `"zcode"` 恰为正确显示名;opencode 工厂 `loaderShared.JSON = true` 为惰性赋值(全包零读取,HEAD 同),作为参考行为镜像保留。
