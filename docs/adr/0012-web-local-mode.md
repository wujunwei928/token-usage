# token-usage web:回环本机模式命令

排行榜(ADR 0011)的流程为社区部署设计:建用户、发 token、客户端上报、网页登录。只想在浏览器里看自己本机消耗、不向任何远程上报的用户,被迫走完这整套——两个终端、手工 token、每次重启重登(会话在内存)。这些步骤没有一步为单机场景服务。

决策内容:

- **新增顶层命令 `token-usage web`**,与 `report` 同级的终端用户动词,而非 `server serve --local`:
  - **安全不变量用命令结构表达**。本机模式的核心是"回环来源免登录",若做成 `serve --local` 的 flag,就与 `--addr` 同处一个命令,`--local --addr 0.0.0.0` 即把免认证面板暴露到网络,只能靠文档、校验与测试去堵组合矩阵。独立命令根本不提供 `--addr`(只有 `--port`),回环绑定是结构保证。
  - **受众分离**。`server` 组是部署运维面(操作员),`web` 是终端用户动词,在 `token-usage --help` 里自然可见。
  - **模式级行为不塞进 flag**。自动建用户、进程内摄入、首启回溯是一组行为变更,flag 装不下且组合语义会一直长。
- **免登录豁免是 opt-in 的构造选项**(`server.WithLoopbackUser`,仅 `web` 命令设置):回环来源的请求解析为隐式本机用户,身份存 DB、重启不丢;`server serve` 的认证行为逐字节不变。豁免不带 cookie,`SameSite` 保护不了免会话身份,故本机模式下所有写请求(POST)额外校验 `Origin`(缺失放行以兼容 curl 与旧客户端,异源 403),阻止本地浏览器里的恶意网页跨站表单 POST 到 127.0.0.1。
- **数据不出进程**:`web` 启动时在进程内跑与 `report` 相同的聚合管道,直接走 store 层 Latest-wins 落库——不发自环 HTTP、不需要 User Token、不挂 `/v1/report`。原始日志与聚合数据都不离开本机(ADR 0001 红线的本机加强版)。进程内回溯沿用 HTTP 路径的同一组上限(550 天 / 20 万行)。
- **默认库 `<user config dir>/token-usage/web.db`**(ADR 0007 命名空间):历史跨次运行持久,且不与部署用的 `leaderboard.db`(工作目录)冲突。
- 仓库有 beyond-upstream 先例(zcode/omp adapter、`report`、`server` 组,ADR 0005/0006/0011),`web` 沿此路径。

## Consequences

- 单机自看的最短路径变成一条命令 + 打开浏览器(交互终端下尽力调起系统浏览器,失败则打印 URL);`add-user`/token/第二终端/重启重登全部消失。
- `internal/server` 的 Web 构造函数增加变参选项;默认行为不变,既有页面与会话测试无需调整。
- 本机模式下 `/` 直接重定向到 `/me`(单用户无榜可排);`server serve` 的榜单首页不变。
- e2e 增加 local-mode 用例(真二进制 `web` 免登录直达 `/me`、数据对账、常驻刷新、`--addr` 结构性拒绝)。
