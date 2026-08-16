# 01 — omp 子命令:pi 格式会话解析与 sidecar 父归属

**What to build:** 终端用户运行 `token-usage omp daily` 即可看到真实的 oh-my-pi token 用量——数据来自 `~/.omp/agent/sessions` 的 pi 格式会话 JSONL(assistant 消息逐次 usage)。口径与 pi 适配器一致(零 token 剔除、totalTokens 回退、首胜去重),omp 特有差异:模型前缀 `[omp] `、sidecar 目录内子会话归并父会话、`OMP_AGENT_DIR`/`--omp-path` 覆盖数据根。daily/monthly/session 三种报表与共享旗标经统一命令框架一并可用。设计依据:`.scratch/omp-adapter/spec.md`、ADR 0006/0009/0010。

**Blocked by:** None — can start immediately

**Status:** ready-for-human

- [x] `token-usage omp daily` 对本机真实 `~/.omp` 数据出报表(2026-07-24 起,glm-5.2/MiniMax-M3 等模型)
- [x] sidecar 子会话(如 `2026-07-30T15-28-14-841Z_019fb3a3-f5f9-…/FuturaDesign.jsonl`)的用量归并到父会话 `019fb3a3-f5f9-…`;session 报表无 `FuturaDesign` 之类伪会话行
- [x] 项目目录名含 `_`(如 `--code-ai-omp_test--`)不误触发父归属,文件自身 stem 生效
- [x] `usage.reasoningTokens` 被忽略且不重复计数(实测 reasoning 已含于 output)
- [x] `OMP_AGENT_DIR` 与 `--omp-path` 覆盖默认根;目录缺失时与其它 adapter 一致的空报表行为
- [x] 空报表 JSON 渲染 null totals(pi 家族形状)

## Comments

2026-08-16 实现完成,验收证据:

- `internal/adapter/omp/`:paths/jsonl/parser/loader/adapter 五文件,contract_test + parser_test 全绿(8 条 fixture 条目:alpha 4 + sidecar 2 + delta 2)。
- 冒烟:`go run ./cmd/token-usage omp daily --offline` 对真实数据输出 7 天行;`omp session` 显示 5 个顶层会话(019fb3a3-5b84 行含并入的 sidecar 用量),无伪会话名。
- 父归属判据 `isSessionStem`:`<YYYY-MM-DD>T` 前缀,见 parser_test 的三个反例用例。
