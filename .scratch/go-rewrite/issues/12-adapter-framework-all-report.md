# 12 — 适配器框架 + All-Report(claude)

**What to build:** Agent Adapter 接口落地(Name / Discover / Load / SupportedReports),common 摄入助手(并行读取、JSONL 管线)收敛为共享基建;All-Report 骨架:裸 `ccusage` 即 all-agent daily(顶层 daily/monthly/weekly/session 同样映射为 all-agent 报表),`--sections daily,weekly,monthly,session` 一次加载输出多报表,`--by-agent` 按 agent 拆分行,`--all` 旗标接受且为 no-op。本工单在 claude-only fixture 上验证:全 agent 报表与 claude 单独报表在只装 claude 的机器上行为一致。

**Blocked by:** 07 — 需全报表形态就位。

**Status:** ready-for-agent

- [ ] Agent Adapter 接口与注册表落地,claude 作为第一个适配器接入
- [ ] 裸 `ccusage` 输出 all-agent daily;claude-only fixture 下与 `ccusage claude daily` 数据一致的 golden
- [ ] 顶层 daily/monthly/weekly/session 映射为 all-agent 报表
- [ ] `--sections` 一次加载输出多报表,Sections 顺序与参考一致
- [ ] `--by-agent` 行按 agent 拆分
- [ ] `--all` 被接受且不改变行为(no-op 兼容)
