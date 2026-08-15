# 01 — 服务端地基:/v1/report 接收与 Latest-wins 落库

**What to build:** 服务端以单二进制启动(随机/配置端口、SQLite 单文件),提供 `POST /v1/report`:校验 Bearer User Token → Device 查找/首次自动绑定(每用户上限 3 台,超出返回明确 409)→ 事务内按 (Device, Report Date) 先删后插 Hourly Usage → 写 reports 审计行 → 返回接受结果与当日 token 汇总。请求体大小上限、按 token 限速、字段严格校验(非法 4xx、脏数据不进库)。测试/运维用:server 自带 CLI 子命令创建用户并签发 User Token(存哈希)。同一次上报重放结果与单次一致(Latest-wins 幂等)。

**Blocked by:** None — can start immediately.

**Status:** resolved

- [x] 启动空库即建全五表(users/tokens/devices/reports/hourly_usage),含 date+tool+model 查询索引
- [x] 种子 CLI 能创建用户并打印一次性 User Token;`/v1/report` 只认 tokens 表中的凭证
- [x] 同一 (Device, Report Date) 第二次上报完全替换第一次,重放幂等
- [x] 新 Device 自动绑定;第 4 台被拒绝且返回可读错误
- [x] 超大请求体、非法字段、超频请求分别被拒,库内无残留
- [x] 端到端测试:真实 HTTP 上报 fixture payload 两次,断言落库内容与响应

## Comments

- 2026-08-15 实现+验收完成。internal/server(store/api/password) + cmd/server;单测 server_test.go:latest-wins/审计/409/校验/限流全过
