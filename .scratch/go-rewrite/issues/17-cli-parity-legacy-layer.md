# 17 — CLI 对等收口:legacy 兼容层全覆盖

**What to build:** CLI 行为级对等(ADR-0002)的最后收口:legacy `codex:daily` 冒号形式归一化为子命令形式(覆盖全部 16 agent);`-a` 在 blocks 上是 `--active`、在其他命令上按旧 `--agent` 短旗标报错;对已移除旗标(`--agent`、`--daily`/`--weekly`/`--monthly`/`--session` 报表开关形式)给出带迁移指引的报错;全命令 `--help` 快照与互斥校验(`--last` vs `--since/--until`/`--sections`)覆盖全部命令。报错文案逐字对齐参考解析器的错误快照。

**Blocked by:** 08 — `-a` 语义依赖 blocks 命令;13 — 冒号归一化需 codex 子命令就位。

**Status:** ready-for-agent

- [ ] 全部 agent 的 `ccusage <agent>:<report>` 冒号形式归一化为子命令形式,输出一致
- [ ] `-a` 在 blocks 上等价 `--active`;在 daily/monthly/weekly/session/statusline 上报错文案逐字对齐
- [ ] `--agent <x>`、`--daily` 等已移除旗标的迁移报错文案逐字对齐(每个命令)
- [ ] 全命令 `--help` 快照测试:cobra 布局允许不同,旗标集合、默认值、别名必须完整
- [ ] 互斥校验覆盖全部命令,报错文案对齐
- [ ] 参数错误退出码与参考一致
