# 02 — 启动时进程内摄入当天用量

**What to build:** `web` 启动时在进程内跑与 `report` 完全相同的聚合管道(全部 agent adapters、相同 dedup 决胜、本地时区切日),复用持久 Device ID 构建当日快照,然后直接走 store 层 latest-wins 落库(每 (Device, Date) 先删后插)——不发自环 HTTP 请求、不需要 User Token、不经过 `/v1/report` 的限速与鉴权。用户视角:`token-usage web` 一条命令,`/me` 上就是今天真实的 token 数字,总量与 `token-usage daily` 可对账(claude 同管道;codex cached 计入榜单口径的既有差异不变)。

**Blocked by:** 01

**Status:** resolved

- [ ] 启动后 `/me` 显示当天真实用量,数字与 fixture 日志下 `token-usage daily` 的当日总量对账一致
- [ ] Device ID 持久且与 `report` 命令共用同一身份文件,设备列表能认出本机
- [ ] 重跑 `web`(重启服务)按 latest-wins 覆盖当日,重放幂等
- [ ] 本地日志为空/不存在时启动不报错,`/me` 呈空数据状态
- [ ] 单测:进程内摄入与经 `/v1/report` 上报同一 fixture 产生的库内逐行一致(同聚合、同落库单元)

## Comments

- 2026-08-18 ingestSnapshots 直驱 Store.ReplaceDay(与 HTTP 摄入同一 Latest-wins 单元);等价性测试断言两路径库内逐行一致 + 手算基线 7080;空快照不清空当日;Device 身份与 report 共用
