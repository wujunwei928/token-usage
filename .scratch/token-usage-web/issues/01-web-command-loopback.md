# 01 — `token-usage web` 命令骨架:回环服务 + 本机用户免登录

**What to build:** 新顶层命令 `web`:首次运行自动播种本机用户(名字默认取 OS 用户名,`--name` 可改,无密码),监听 127.0.0.1 的 `--port`(默认 8787),回环来源的请求直接视为该用户——浏览器打开即落在 `/me`,无登录、无 token、无第二终端。**命令不提供 `--addr`**,回环绑定是结构性不变量。`server serve` 及其认证行为零变化;`/v1/report` 仍要求 Bearer token。随票落盘 ADR 0012(命令面与免登录豁免的决策)并接线 README。

**Blocked by:** None — can start immediately.

**Status:** resolved

- [ ] `token-usage web` 启动后浏览器打开 `http://127.0.0.1:8787` 直达 `/me` 个人面板(空数据状态),全程无密码、无 token
- [ ] 本机用户首次运行自动创建、之后复用同一身份(数据与 `--db` 同路径持久,默认 `<config>/token-usage/web.db`)
- [ ] 免登录豁免只对回环来源生效,且只存在于 `web` 命令;`server serve` 路径的 `requireUser` 行为与从前逐字节一致
- [ ] `web` 命令面只有 `--port` `--db` `--name` `--pricing`,不存在能改变监听地址的 flag
- [ ] 裸 `token-usage web --help` 输出与既有命令风格一致;ADR 0012 与 README(server README 快速开始区)已更新
- [ ] 单测:回环请求豁免生效、非回环来源(伪造 RemoteAddr)不豁免

## Comments

- 2026-08-18 回环+免登录落地:server.WithLoopbackUser opt-in 豁免(serve 路径零变化)、web 命令无 --addr、ADR 0012 与双 README 接线;单测 web_local_test.go + web_test.go,真二进制冒烟通过
- 2026-08-18 review 追补:`/` 在本机模式重定向到 /me(WithLocalRoot),免 cookie 身份的 POST 加 Origin 同源校验(异源 403),ensureLocalUser 区分 ErrNoRows 与 DB 错误;flag 面在 03/04 落地后含 --since/--refresh,属后续票的演进而非违背本票清单
