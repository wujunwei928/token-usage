# 01 — cline 会话消息适配器与计量口径

**What to build:** `internal/adapter/cline` 包:发现 `~/.cline/data/sessions/<id>/<id>.messages.json`(`CLINE_DATA_DIR` 覆盖),按调研口径解析(缓存扣减、cost 字段、manifest 回退),按兄弟标准四文件布局,经 `cline daily/monthly/session` 子命令出报表;Detected 走 HasData 口径。

**Status:** ready-for-human

- [x] `inputTokens − cacheRead − cacheWrite`(下限 0)为 input 口径,cache 两类单独保留,总量 = inputTokens + outputTokens
- [x] `metrics.cost` 非负有限数(含数字字符串)→ provider-reported costUSD;auto 优先、calculate 恒按 token、display 信任之缺失记 0;零 token 带 cost 的消息保留
- [x] manifest 回退:model/provider 播种、`modelInfo` 逐消息覆盖;`workspace_root` > `cwd`;sessionId:文件 > manifest > stem
- [x] 包布局对齐 17 个兄弟 adapter:`adapter.go`/`paths.go`/`loader.go`/`parser.go` + `contract_test.go`
- [x] 共享 `findPricedModel` 解析定价候选(bare id → provider/id),cost 与 missing-pricing 单次口径
- [x] 空 JSON 报表 `totals: null`(totalsNullEmpty,与 pi/omp/zcode/qwen 一致)
- [x] 契约测试(合成 sessions fixture)+ parser 口径回归(缓存扣减/溢出 clamp、cost 字段四态、manifest 回退、cost mode × missing pricing)

## Comments

2026-09-08 实现:`go test ./internal/adapter/cline/` 7 例全绿(1 契约 + 6 口径);`go vet` 干净。
