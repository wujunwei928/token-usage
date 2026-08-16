# 02 — omp 纳入 all-report、roster 与文档 golden

**What to build:** omp 按 ADR 0006 的超越上游模式默认纳入:roster 追加、all-report spec 注册(IncludeProjectPath)、`Detected:` 行显示 `oh-my-pi`(HasData 口径)、legacy 冒号别名 `omp:daily` 可用、README/README.zh-CN/CONTEXT.md 文档、脱敏 fixtures 与 golden 用例钉住 CLI 行为。

**Blocked by:** 01

**Status:** ready-for-human

- [x] 裸 `token-usage` 的 `Detected:` 行出现 `oh-my-pi`(本机实测:`Claude, Codex, Hermes, oh-my-pi, OpenCode, pi-agent, zcode`)
- [x] roster 末尾追加 `omp`(zcode 之后),registry_test 同步
- [x] `token-usage omp:daily` 冒号别名可路由;排行榜 snapshot 经 roster 自动上报 omp 维度(无需服务端变更)
- [x] README.md / README.zh-CN.md 增加 oh-my-pi 小节;CONTEXT.md 词汇表(Agent Adapter 示例、Detected 的 HasData 名单)更新
- [x] golden 用例 `omp-daily` / `omp-daily-json` / `omp-session` 用 fixture 数据钉住输出与退出码

## Comments

2026-08-16 实现完成,验收证据:

- golden 三例由本仓库二进制按 golden_test 环境契约生成并人工核算:daily 四行 token 逐项对账(01-02 行 input 2,100 = 1000+100+600+400 等)、session 行 2,610 = 父会话 1,610 + sidecar 1,000、claude-sonnet 缺 costUSD 条目按内嵌单价补 $0.003。
- `go test ./internal/golden/ -run TestGolden/omp` 通过;全量 `go test ./...` 通过。
- 排行榜:未改 server 端;`internal/report` snapshot 经 `common.Roster()` 自动覆盖(ADR 0006 的既有结论)。

2026-08-16 code-review(Standards/Spec 双轴)后补齐:

- 修复 `jsonl.go` 注释措辞:"shared with the pi adapter" → "mirrored from"(复制而非共享,改 pi 不传导)。
- 新增 golden `omp-empty-json`(`OMP_AGENT_DIR` 指向缺失目录 + `--json`):钉住空报表 `{"daily": [], "totals": null}` 与退出码 0——填补 Spec 轴唯一发现(空报表 null-totals 此前无自动化覆盖)。
- 备忘不改:ADR 0009 Consequences 的"HasData 仅 qwen/zcode/opencode"为时点记录,活性词汇表以 CONTEXT.md 为准(已含 omp)。
- 回归:`go test ./internal/golden/ -run TestGolden/omp`(4 例)与 `./internal/adapter/omp/` 全绿。
