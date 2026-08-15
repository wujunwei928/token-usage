# 04 — 价格模块与 Leaderboard v1(无筛选)

**What to build:** 服务端价格模块:启动时加载价格表 JSON(LiteLLM 快照做种子 + 模型家族前缀兜底估算,来源标 official/estimated),移植 CLI 域计费语义——input/output/cache read 单价、cache write 5m 按 1×input、1h 按 2×input、200K 边际分层、OpenAI long-context 整档切换、fast 倍率。`GET /` 渲染 Leaderboard 默认视图:所选范围先固定为"今天",排名卡片含头像、昵称、城市、模型徽标(按 model breakdown)、token 数、Cost Estimate、设备数;成本查询时现算不落库。未登录访客可直接浏览。

**Blocked by:** 02(要有真实上报数据可排)。

**Status:** resolved

- [x] 价格表 JSON 独立于代码,启动加载;调价重启即生效
- [x] 已知模型成本与手工按单价表计算一致(含 5m/1h 缓存差异定价)
- [x] 未知模型按家族前缀兜底并在页面标注 estimated
- [x] `/` 页按当日 token 排名,卡片字段齐全,无需登录
- [x] 模型徽标来自该用户当日 model breakdown
- [x] 端到端测试:多用户上报 fixture 后,页面聚合数字与 Cost Estimate 正确

## Comments

- 2026-08-15 实现+验收完成。pricing.go(种子+model-prices.json 覆盖+家族兜底 estimated)+ Leaderboard v1;TestLeaderboardPageAndFilters/TestPricingCostMath
