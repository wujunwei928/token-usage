# 单二进制:排行榜 server 并入 token-usage 命令树

排行榜服务端此前是独立二进制 `token-usage-server`(`cmd/server`,裸 `flag` 包自路由):与客户端 CLI 同一 module、同一 `go.mod`,依赖高度重叠(sqlite 驱动早已因 zcode adapter 进入 CLI 侧,实测 CLI 20M / server 19M,合并后仅增 1–2M)。双二进制在构建、分发、文档与心智上割裂——用户装一个工具要拿两个文件、记两套命令风格(cobra `--flag` vs flag 包 `-flag`),客户端与服务端版本还可能偏斜。用户决定:合并为一个二进制。

决策内容:

- **单二进制单入口**:只保留 `cmd/token-usage`。server 生命周期命令收进 `server` 命令组(`internal/cli/server.go`):`token-usage server serve` / `server add-user` / `server add-token` / `server dump-pricing`,实现仍全部留在 `internal/server` 包,`cmd/server` 删除。
- **裸 `token-usage server` 只打印帮助**,沿袭无参数默认 `serve` 的旧行为被有意放弃:误敲命令不应意外起一个监听进程。
- **`serve` 默认监听 `127.0.0.1:8787`**(与旧二进制一致,但在此升格为明确的安全姿态):合并意味着 server 能力随每个客户端分发,loopback 默认保证在任何机器上误启动都不对外暴露端口;部署到服务器时由管理员显式 `--addr 0.0.0.0:8787`(建议配 TLS 反代)。
- **flag 迁 cobra/pflag `--` 风格**(`-addr` → `--addr` 等),默认值全部不变;这是对 server 管理员的破坏性变更,项目尚未发布稳定版,直接切换不留过渡壳。

## Consequences

- 客户端与服务端永远同版本(单一版本线),无版本偏斜;发布、安装、文档只涉及一个文件。
- server 代码随所有客户端分发(约 +1–2M)。代码不执行即无运行时面,但若未来公开分发且介意攻击面,可再评估按 build tag 拆分——目前自托管社区场景不构成边界。
- 管理员需改用 `token-usage server ...` 与 `--flag` 风格;旧 `token-usage-server` 不再构建。
- 客户端命令面不变:`token-usage report` 及其 crontab 定时器(记录的是可执行文件路径 + `report` 子命令)不受影响。
- e2e 全链路改为单二进制自扮演双端(`server serve`/`server add-user` + `report`),比双二进制更贴近真实分发形态;`scripts/demo.sh` 同步只构建一个产物。
- 两个 bounded context(CLI 域 / 排行榜域)的词汇表与边界不变——合并纯属实现层,不进 CONTEXT.md。
