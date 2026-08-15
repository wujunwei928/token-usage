# 08 — Anomaly Flag 防刷(v1)

**What to build:** 落库时检查单设备单日总 token,超过阈值(初始 10 亿,配置可调)给该 Device 当日 Hourly Usage 打 Anomaly Flag:Leaderboard 聚合排除被标记行(所有筛选与口径下一致),本人 Dashboard 仍完整可见并显示被标记提示;reports 审计表保留完整上报轨迹(频次、条数、总量)。阈值外的一切行为不变。

**Blocked by:** 05(榜单聚合),06(本人可见性)。

**Status:** resolved

- [x] 超阈值设备的当日数据不出现在任何筛选组合的 Leaderboard
- [x] 同设备未被标记的其他日期正常上榜
- [x] 本人 Dashboard 数据完整且有可见的标记提示
- [x] 审计表可还原该设备当日上报轨迹
- [x] 端到端测试:构造超阈值 fixture,断言榜单排除 + 本人可见

## Comments

- 2026-08-15 实现+验收完成。ReplaceDay 阈值(1e9)置 flagged;榜单排除、本人可见+提示;TestAnomalyFlag
