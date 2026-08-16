# Spec: 统一重构收尾——文档腐化与残留清理

Status: done

> 来源:2026-08-16 code-review(Standards + Spec 双轴)对未提交的统一 Adapter 接口(`.scratch/unified-adapter-interface`,ADR 0009)与统一命令框架(`.scratch/agent-command-framework`,ADR 0010)改动的审查发现。全部为收尾性质:注释/ADR 与最终代码不一致、死代码、grok 名位残留、行为变更未落盘。无代码行为变更。

## Problem Statement

`LoadRequest` 中途收窄(仅 `Shared`,删除 `Pricing` 字段)与 grok 全量删除两项修订的收尾清理不彻底:

1. **ADR 0009 决策正文与自家 Consequences 矛盾**:`docs/adr/0009-unified-agent-adapter-interface.md:3` 仍写"`LoadRequest` 仅含 `Shared` 与 `Pricing`",而同文 Consequences 已记录 `Pricing` 移除、代码(`internal/adapter/common/adapter.go:30-32`)只剩 `Shared`。违背该 spec issue 08"ADR 0009 与最终代码逐条核对"的验收要求。(双轴审查两侧同时命中,为本次最重发现。)
2. **5 个 adapter 注释提及已删除字段**:`internal/adapter/droid/adapter.go:13`、`hermes/adapter.go:13`、`zcode/adapter.go:18`、`codebuff/adapter.go:13`、`qwen/adapter.go:17` 仍写 "Pricing is unused / in the request"。
3. **残缺注释**:`internal/adapter/opencode/report.go:11-14`("…the local enum is / an alias of core.ReportKind. / adapter-local use.")语序破碎。
4. **grok 显示名席位残留**:`internal/adapter/all/report.go:19` AgentLabel 仍有 `"grok": "Grok"`;ADR 0010(Q5)要求矩阵/显示名名位全清,`errors.go`/`normalize.go` 已清、此处漏网。
5. **死代码**:`internal/adapter/common/reportjson.go:27-29` `ReportFromRows` 全仓库零调用,仅转发 `AgentReportJSON(rows, kind, false, totalsNullEmpty)`。
6. **行为变更只记在代码注释**:`summarizeSessionsByActivity` 对 session key 排序(HEAD 的 opencode 为插入顺序),仅见 `internal/adapter/common/pipeline.go:50-54` 注释;spec 实施修订记录与 ADR 均未提及。

## Solution

逐项清理,零代码行为变更:ADR 0009 决策正文与最终代码对齐;5 处 adapter 注释改写;opencode 注释补全;删 AgentLabel 的 grok 条目与 `ReportFromRows`;session 排序变更补记入 unified-adapter-interface spec 的实施修订记录。

## User Stories

1. 作为代码导航者(人或 AI),我想让注释与 ADR 读到的即是现行契约,这样不会按中途设计理解接口
2. 作为维护者,我想删掉零调用的 `ReportFromRows`,这样 reportjson.go 不再携带死代码
3. 作为 grok 观望者,我想让 grok 名位彻底清空(复活靠 git),这样矩阵/显示名不再有幽灵席位
4. 作为 opencode 报表消费者,我想让 session 排序变更落盘到 spec 修订记录,这样变更历史可追溯

## 验收清单

- [x] ADR 0009 决策正文、Consequences、`common/adapter.go` 三方一致(仅 `Shared`)
- [x] 5 处 adapter 注释不再提及 `Pricing` 请求字段
- [x] `opencode/report.go` 注释完整可读
- [x] `all/report.go` AgentLabel 无 grok 条目;142 golden 字节不变
- [x] 删除 `ReportFromRows` 后 `go build ./...` / `go vet ./...` / 全量测试绿
- [x] `.scratch/unified-adapter-interface/spec.md` 实施修订记录补记 session 排序变更

## 完成记录(2026-08-16,TDD 修复轮)

六项全数完成之外,同轮顺手修复了第二轮 review(meta-review 复核后)的新发现,均零行为变更:

1. **第 6 处 Pricing 注释**:统一接口自身 `common/adapter.go` 的 `LoadEntries` 文档注释仍写 "take it from the request"——已改写为工厂闭包表述。
2. **roster-index 注释背离 ADR 0009**:`all/spec_codex.go`、`all/spec_opencode.go` 头注释引用已废除的魔法下标("roster index 1/2, after claude (0)")——已改为按名注册表述。
3. **opencode adapter 注释失实**:`opencode/adapter.go` 称 "the loader uses that flag [JSON] to narrow reads"——loader 实际按 `--since/--until` 窗口收窄,`shared.JSON` 全包零读取(HEAD 亦然);注释已改真,惰性赋值作为参考行为镜像保留。
4. **story 11 词汇残留**(未入任何清单的新发现):`goose/report.go` 死常量 `ReportDaily…ReportSession`(零引用)、`copilot/report.go` 整文件仅剩别名块(仅 parser_test 用 `KindDaily`)、`opencode/report.go` 死 `Kind*` 常量块——全数删除,parser_test 改用 `core.KindDaily`。
5. **微瑕**:`report/snapshot.go` 循环内 `tool := name` 无意义别名删除。

验收:`go build` / `go vet` / 全部包测试绿;142 golden 字节不变(527.7s,注意 `go test` 默认 600s 超时不够,需 `-timeout` 显式放宽)。第二轮 review 的判断题发现(S3 名单五处散布、S4 渲染旗标 Data Clump、S5 `lastPeriodUnit`)未修复,已记入 `.scratch/duplication-consolidation/spec.md` 待 Q1/Q2 一并定案。

## Out of Scope

- `internal/config/config.go:266` 的 grok(上游 `BUILT_IN_AGENT_NAMES` 镜像,ADR 0010 Q5 明确不动)
- 重复逻辑收敛、工厂闭包注入补全(见 `.scratch/duplication-consolidation/spec.md`)

## 建议的 issue 拆分(留待 /to-tickets)

1. 注释与 ADR 对齐:ADR 0009 正文 + 5 处 adapter 注释 + opencode 残缺注释
2. 删除项:AgentLabel grok 条目 + `ReportFromRows`(golden + 全量测试护航)
3. 落盘:session 排序变更补记入 unified-adapter-interface 实施修订记录
