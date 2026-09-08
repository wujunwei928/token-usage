# 02 — cline 纳入 all-report、roster 与文档 golden

**What to build:** cline 按 ADR 0006 的超越上游模式默认纳入:roster 追加、all-report spec 注册(IncludeProjectPath)、`Detected:` 行显示 `Cline`(HasData 口径)、README 双语小节、CONTEXT.md 词汇表、脱敏 fixtures 与 golden 用例钉住 CLI 行为。

**Blocked by:** 01

**Status:** ready-for-human

- [x] roster 末尾追加 `cline`,registry_test 与 roster_coverage_test 守卫四处名位(report.go label、rosterOrder、register.go、normalize.go agentNames)
- [x] `all/spec_cline.go` 注册 AdapterSpec(Profile + IncludeProjectPath)
- [x] README.md / README.zh-CN.md 增加 Cline 小节(数据源、CLINE_DATA_DIR 覆盖、计量口径);CONTEXT.md 的 Agent Adapter 示例与 Detected HasData 名单同步
- [x] golden 用例 `cline-daily` / `cline-daily-json` / `cline-session` / `cline-empty-json` 用 fixture 数据钉住输出与退出码

## Comments

2026-09-08 实现完成,验收证据:

- golden 四例由本仓库二进制按 golden_test 环境契约生成并人工核算:daily 01-02 行 input 1,140 = 740(=1000−250−10)+ 400(=500−80−20)钉住缓存扣减;01-03 行 670/190/30 全部经 manifest 回退(model/provider/workspace_root),含零 token 带 cost 0.003 的保留条目;claude-sonnet-4 缺 costUSD 条目按内嵌单价计 $0.0083325(740×3e-6 + 400×15e-6 + 10×3.75e-6 + 250×0.3e-6),glm-5.2 带 costUSD 条目直接采用(0.0061/0.02),missing pricing WARN 仅指向无 cost 的 glm-5.2 条目。
- `cline-empty-json` 钉住空报表 `{"daily": [], "totals": null}` 与退出码 0(此即补设 `totalsNullEmpty: true` 的回归)。
- 本机 Windows 下 `internal/golden` 的 TestMain 构建无 `.exe` 后缀二进制导致 exec 失败(既有环境问题,omp 四例同样失败),故按契约逐 case 字节比对验证:4/4 OK;CI(Linux)走原生 golden_test。
