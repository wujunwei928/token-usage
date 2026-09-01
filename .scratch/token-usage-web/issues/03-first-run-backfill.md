# 03 — 首启自动回溯

**What to build:** `web` 检测到本机用户在当天之前没有任何落库数据(典型:全新 `--db` 首次运行)时,自动做一次性回溯——复用客户端回溯语义把自 `--since`(默认近 30 天)至今每天的快照逐日落库,空数据的天自动跳过、不触碰已有数据,重跑幂等。用户视角:全新库第一次 `token-usage web` 启动完成时,`/me` 的 30 天趋势图直接是满的,无需任何手动回填命令。

**Blocked by:** 02

**Status:** resolved

- [ ] 全新 DB 首次启动后,`/me` 近 30 天趋势与 fixture 日志的逐日总量对账一致
- [ ] `--since YYYY-MM-DD` 可改回溯起点,受既有 550 天 / 20 万行回溯上限约束,超限给出与 `report --since` 一致的报错
- [ ] 无数据的天跳过且不清空;已有数据的天重跑覆盖(latest-wins),整段回溯可安全重复
- [ ] 非首启且未显式传 `--since` 时不触发回溯,启动路径与 02 一致(2026-09-01 修订,见 Comments)
- [ ] 单测:首启判定、空日跳过、幂等重放

## Comments

- 2026-08-18 store.HasUsageBefore 触发首启回溯(BuildBackfill 逐日 latest-wins);--since 校验与 550 天上限;幂等重放与非首启跳过均有测试
- 2026-08-18 review 追补:进程内回溯补上 20 万行上限与 550 天的精确日数校验(与 HTTP 路径同一组 server 导出常量、同文案);新增 TestBackfillReplayReplacesDays 覆盖整段重放的 latest-wins 覆盖
- 2026-09-01 行为修订(用户发起):显式 `--since <date>` 现在无条件重放该区间——动机是"某天没开 web 导致该天缺数据"时,原语义下只能重置 web.db 才能补回。重放沿用既有 latest-wins/空日跳过保证(其他设备与区间外的行不动,TestBackfillExplicitSinceReplaysDespiteHistory 固化);显式空串等价于未传。第 4 条验收标准相应细化为"非首启且未显式传 `--since` 时不触发回溯"。spec.md 数据 bullet 已同步。
