# 01: 双主题设计系统与三态切换

**What to build:** 全站在亮色下呈现 token 化的新设计系统(双主题色板、字阶、间距/圆角/阴影阶),顶栏出现 亮/暗/自动 三态切换按钮:点击即全站换肤,选择记忆在 localStorage,刷新页面无闪白(FOUC)。亮、暗两主题下现有全部页面(排行榜、我的 Token、价格表、数据说明、登录/注册、设置)均正确着色、无破版。已知中间态:ECharts 图表仍为固定配色,由 03 票统一。

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] 亮/暗/自动三态切换生效;显式选择优先于系统偏好,自动态跟随 prefers-color-scheme
- [ ] 切换与刷新均无 FOUC(主题脚本先于样式生效)
- [ ] 暗色主题下各页面(含表单、表格、榜单、flash 提示)可读,无破版
- [ ] 样式表内颜色全部引用 design token,无散落硬编码色值
- [ ] `go test ./internal/server/... ./internal/e2e/...` 全绿;测试断言的元素 ID 与文案未被改动
