# 03: 图表主题同步

**What to build:** 亮暗切换时,Dashboard 五张图表(小时时间线、30 天趋势、按工具、按模型、Token 构成)无需刷新即时跟随换肤:系列配色、坐标轴标签、网格线、tooltip 底色随主题走;窗口 resize 行为不回退。图表调色板与 CSS design token 单一来源(运行时读取),不再在脚本里散落硬编码色值。

**Blocked by:** 01(主题机制与 token 就位)

**Status:** ready-for-agent

- [ ] 三态切换(含自动态跟随系统)时五张图表全部同步换肤,无需手动刷新
- [ ] 切换以重建实例实现,不重发任何数据请求
- [ ] 调色板来源于 CSS token;数据系列色阶映射自 token,无独立于设计系统的色值
- [ ] 窄拖窗口后图表尺寸正确(resize 处理不回退)
- [ ] `chart-hourly`、`dash-data` 等断言 ID 不变,`go test ./internal/server/... ./internal/e2e/...` 全绿
