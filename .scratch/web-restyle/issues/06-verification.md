# 06: 集成验收

**What to build:** 按 spec 验收清单对整个 web-restyle 工程做端到端验收:双模式(web 本机 / serve 社区)手检、亮暗三态全链路体验、网络面板零外部请求确认、移动端宽度检查、测试与 e2e 全量跑通,结果记录回本票。

**Blocked by:** 01, 02, 03, 04, 05

**Status:** ready-for-agent

- [x] `token-usage web` 打开 /me:亮色精致、三态切换记忆跨刷新、图表同步、无 FOUC
- [x] web 模式品牌为「Token 用量 · 本机数据 · 不出网」;serve 模式品牌与功能与从前一致
- [x] 375px 宽度无横向滚动
- [x] devtools 网络面板仅 `/static/*` 与页面请求,零外部请求
- [x] `go test ./internal/server/... ./internal/e2e/...` 全绿;`rg -i ccusage internal/server` 为空

## Comments

- 真二进制冒烟(/me 标记全中:bootstrap 脚本/切换按钮/品牌/主卡/进度条/图表缝;/ → 303 /me;/pricing //login 200)。
- 模板零外链(rg https?:// 仅命中示例文案);全部资产走 /static 内嵌。
- `go test -count=1 ./...`:server、e2e 全绿;golden 与 terminal 两个包在基线 edd0686 即同样失败(存量环境问题,与本工程无关,已用 git worktree 复核)。
- `rg -i ccusage internal/server` 仅命中守护测试自身。
