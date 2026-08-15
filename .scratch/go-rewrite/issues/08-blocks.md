# 08 — blocks:5 小时计费窗口

**What to build:** `ccusage blocks` 完整对等:5 小时窗切割算法(session 首条目 `floor_to_hour` 起算,`since_start > duration` 或 `since_last > duration` 切割)、空闲超窗的 gap 灰块、`is_active` 判定、Burn Rate(token/min、非 cache token/min、成本/小时)、到窗口结束的 projection、`--token-limit` ≥80% 告警;旗标 `--active`/`-a`、`--recent`(默认 3 天)、`--token-limit <n|max>`、`--session-length <hours>`(默认 5)。

**Blocked by:** 03 — 窗口成本需计价体系;05 — blocks 表格需渲染器(<120 列紧凑)。

**Status:** ready-for-agent

- [ ] 窗口切割算法对拍参考实现测试值的单测(切割边界、gap 插入、active 判定)
- [ ] gap 灰块行与 ACTIVE 标记的表格 golden
- [ ] Burn Rate 三指标与 projection 数值、格式化对齐
- [ ] `--token-limit` 达 80% 时告警显示;`max` 取值行为对等
- [ ] `--active` / `-a`、`--recent`、`--session-length` 全旗标 golden(表格与 `--json` 两种形态)
