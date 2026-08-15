# 06 — Dashboard `/me` 个人仪表盘

**What to build:** 登录用户的个人统计页:8 个指标卡(当日消耗、Cost Estimate、缓存命中、连续活跃天数等,口径对齐截图)、当日 hour×tool 堆叠时间线、近 30 天用量与成本双柱图、按 Tool / model / Token 构成 / Device 的分布图、设备列表(标签 + 最近同步时间)。图表由模板内联 JSON 喂 ECharts,筛选整页刷新。历史趋势用服务端已存的各日 Hourly Usage(测试可直接向 server 上报多日 fixture payload 构造)。

**Blocked by:** 02(数据来源),03(登录态)。

**Status:** resolved

- [x] 未登录访问 `/me` 重定向登录;登录后可见本人全部数据
- [x] 8 指标卡数值正确,口径在页面上可解释
- [x] 当日时间线按小时×工具堆叠,与上报 hours 一致
- [x] 近 30 天图表多日数据正确、缺数据日显示为空
- [x] 四类分布图与设备列表(含最近同步时间)正确
- [x] 端到端测试:多日 fixture 上报后断言页面关键数字

## Comments

- 2026-08-15 实现+验收完成。Dashboard /me:8 指标卡/小时×工具时间线/30 天双轴/四类分布/设备表;TestAccountFlowAndDashboard + 浏览器截图验收
