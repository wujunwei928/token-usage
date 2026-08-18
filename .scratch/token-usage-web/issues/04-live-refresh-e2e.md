# 04 — 运行中保持新鲜 + local-mode e2e

**What to build:** 服务常驻期间按 `--refresh` 间隔(默认 15 分钟)在后台重新聚合并按 latest-wins 覆盖当日,刷新期间页面不阻塞、失败只告警不影响服务;`/me` 页面提供手动刷新入口。并在 `internal/e2e` 增加 local-mode 全链路用例:真实二进制跑 `token-usage web`,断言免登录直达 `/me`、数据与 fixture 日志对账、常驻刷新后新写入的日志用量可见。用户视角:起一次服务放着,当天数字自己长。

**Blocked by:** 02

**Status:** resolved

- [ ] 超过刷新间隔后当日数据自动更新,页面无需重启服务
- [ ] `--refresh` 可调;`0`/负值给出明确报错而不是 panic 或死循环
- [ ] 刷新与请求并发安全;刷新失败(如日志暂时不可读)记录告警、服务与页面不受影响
- [ ] `/me` 有手动刷新入口,触发后立即反映最新聚合
- [ ] `internal/e2e`:真实二进制 `web` 起服务 → `/me` 免登录、数据对账、追加 fixture 后(手动刷新或触发间隔)新用量可见

## Comments

- 2026-08-18 --refresh 默认 15m(非正值报错)后台重聚合,refreshMu 串行化;WithRefresh 挂 POST /refresh + /me 刷新按钮(仅本机模式);e2e TestWebLocalEndToEnd 覆盖免登录/回溯/追加日志后刷新可见,--addr 结构性拒绝
