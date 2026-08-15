# 02 — 客户端:`ccusage report` 命令

**What to build:** 一条命令完成采集到上报:复用 CLI 域全部 16 个 Agent Adapter 与 dedup(与本地报表同口径),按客户端本地时区(复用现有 timezone 选项)过滤今天的 entries,聚合为 Hourly Usage((hour, tool, model) 五类计数:input/output/cache read/cache write 5m/cache write 1h,无拆分对象时全部计 5m),连同 Device ID 一起以 Report Snapshot 全量上报。首次运行生成随机 UUID 与设备标签(默认 hostname)持久化到用户配置目录。server 地址与 User Token 按 flag > 环境变量 > ccusage.json 解析。成功输出当日 token 总数与设备数;server 不可达时清晰报错、退出码非零、无副作用。

**Blocked by:** 01(需要可用的 /v1/report)。

**Status:** resolved

- [x] 首次运行生成并持久化 Device ID 与标签;再次运行复用
- [x] fixture 含 claude 日志时,上报的 hours 数组各计数与本地 `ccusage` 报表口径一致
- [x] cache write 按 5m/1h 拆分;无拆分对象时全计 5m
- [x] 只含今天(客户端本地时区)的 entries;昨天/明天的 fixture 不进 payload
- [x] flag / 环境变量 / ccusage.json 三种配置方式都生效,优先级正确
- [x] 重跑命令,服务端当日数据被覆盖(Latest-wins)
- [x] server 不可达:报错信息含地址,退出码非零
- [x] 端到端测试:起真实 server + fixture 日志目录跑真实命令,断言落库与 stdout

## Comments

- 2026-08-15 实现+验收完成。internal/report + internal/cli/report.go;e2e_test.go 真二进制全链路:16 适配器当日聚合、device.json、三级配置、latest-wins、dry-run
- 2026-08-15 追加:回溯上报能力(`--since YYYY-MM-DD`,单请求多日 payload,服务端 550 天/20 万行上限);claude 采集改走 daily 管道(LoadDailyDetailEntries)与 `ccusage daily` 严格同口径。真机半年数据(2.27B tokens/58 天)对账:claude 历史天逐日精确一致,codex 差额逐日恒等于当日 cached tokens(榜单含缓存口径),未解释差异 0。
