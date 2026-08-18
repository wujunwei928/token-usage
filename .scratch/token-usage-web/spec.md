# Spec: token-usage web(本机模式)

Status: ready-for-agent

## Problem Statement

榜单(ADR 0011)的定位是社区部署:上报、排名、登录。但"只想在浏览器里看自己本机消耗、不向任何远程上报"的用户,今天要走过头流程——`server add-user` 建带密码的用户、另一个终端手动 `report` 自环 HTTP 上报、浏览器登录,且 `serve` 每次重启都会丢内存会话要求重新登录。这些步骤没有一步是为单机场景服务的。

## Solution

新增顶层命令 `token-usage web`:一条命令 + 打开浏览器,即为全部操作。

- **命令面**:只有 `--port`(默认 8787)、`--db`(默认 `<config>/token-usage/web.db`,数据跨次运行持久)、`--name`(本机用户名,默认取 OS 用户名)。**没有 `--addr`**——监听地址硬编码 127.0.0.1,回环绑定是结构性不变量而非运行时条件,免认证 dashboard 不可能被 flag 组合暴露到网络。`server` 组保持纯部署面。
- **身份**:首次运行自动播种本机用户(无密码),回环来源的请求直接视为该用户——`/me` 免登录,重启不丢(身份来自 DB 而非会话)。
- **数据**:启动时在进程内复用与 `report` 完全相同的聚合管道(全部 adapters + 相同 dedup 口径)与持久 Device ID,构建当日快照后直接走 store 层 latest-wins 落库——不发自环 HTTP、不需要 User Token。首次运行检测到无历史数据时自动回溯(默认近 30 天,`--since` 可改,受 550 天回溯上限约束)。服务常驻期间按 `--refresh` 间隔(默认 15 分钟)重新聚合并覆盖当日。
- **安全红线**:免登录豁免只存在于 `web` 命令,且仅在回环监听下成立;`server serve` 的认证行为零变化。`/v1/report` API 行为不变(仍要求 Bearer token)。

决策记录:ADR 0012(随首张票落盘)。原始日志照 ADR 0001 红线永不离开本机——本模式下连聚合数据也不离开本机。

## User Stories

1. As a 单机用户, I want `token-usage web` 一条命令就起服务并在浏览器看到 `/me`, so that 不建用户、不粘 token、不开第二个终端。
2. As a 单机用户, I want 免登录直达个人面板且重启服务不丢身份, so that 日常就是"起服务、看一眼"。
3. As a 单机用户, I want 数据口径与 `token-usage daily` 完全一致(同管道同 dedup), so that 网页数字和本地报表可对账。
4. As a 单机用户, I want 首次启动自动回填近 30 天, so that 30 天趋势图第一次打开就是满的。
5. As a 单机用户, I want 服务常驻时数据自动保持新鲜, so that 不用重启才能看到新用量。
6. As a 部署者, I want `server serve` 的认证与暴露行为与从前完全一致, so that 本机模式不给部署面引入任何新风险。
