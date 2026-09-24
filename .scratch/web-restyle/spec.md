# Spec: web 面板样式重设计(web-restyle)

Status: ready-for-agent(方案与拆票均已确认,票据已发布至 issues/)

## Problem Statement

`token-usage web`(本机面板,ADR 0012)功能已就绪,但界面仍是排行榜时代的一次性样式:133 行浅色单主题 CSS、无暗色模式、仅一个 900px 断点;单用户本机场景下 topbar 仍挂着「Token 排行榜」品牌与无意义的排行榜导航;模板中残留 4 处 rebrand 前的 `ccusage` 文案(settings.html ×3、about.html ×3、index.html ×1)。视觉质感、可读性(大数字、图表轴标签、表格对齐)与一致性(各页间距/圆角/色板漂移)三个维度都欠打磨。

## Decisions(三轮 grilling 已定格)

| # | 决策 | 备选与取舍 |
|---|------|-----------|
| D1 | 目标 = 视觉质感 + 可读性/信息密度 + 一致性(设计系统化),三者兼修 | — |
| D2 | 改动面 = `style.css` 重写 + 模板 HTML 结构微调 + `app.js` 图表主题 | 仅 CSS 天花板低;整体重做风险大 |
| D3 | 设计语言 = 精致亮色双主题,延续 indigo(#5b5bd6 系)品牌色 | 暗色优先终端风会带偏社区页气质;编辑风极简对高密度榜单偏空 |
| D4 | 视角 = 以本机模式 `/me` Dashboard 为主战场;社区页共享设计系统顺带受益,不单独优化 | web 模式下 `/` 永远重定向 `/me`,排行榜首页仅 `server serve` 可见 |
| D5 | 本机模式品牌 = 「Token 用量」,副标注「本机数据 · 不出网」;社区模式保持「Token 排行榜」不变 | 词条已入 `server/CONTEXT.md`(Local Panel / 本机面板) |
| D6 | 暗色 = 亮/暗/跟随系统三态切换,顶栏按钮,localStorage 记忆,ECharts 同步换肤 | 仅自动跟随无法固定亮色;仅手动不感知系统 |
| D7 | 动效 = 克制过渡:纯 CSS hover/过渡 + ECharts 自带入场,不自写滚动 reveal/数字滚动 | 工具气质,不抢戏 |
| D8 | 资源 = **零外部请求**红线:系统字体栈、内嵌 CSS/JS、图标用内联 SVG | CDN 字体每次打开向第三方发请求,违背本机模式数据不出网哲学 |

不新增 ADR:样式决策全部可逆,不满足"难逆转"条件;本 spec + CONTEXT.md 词条即为决策记录。

## Solution

### 1. 设计系统(style.css 全量重写)

- **Design tokens**:`:root` 定义双主题色板(亮/暗各一套:背景、卡片、墨色、muted、accent、accent-soft、线框、警示、金/银/铜)、4px 基准间距阶(4/8/12/16/24/32)、圆角阶(8/12/16)、两档 elevation 阴影(亮色用影,暗色用边框提亮替代)。
- **字阶**:12/13/15/17/22/28 六级;数字统一 `font-variant-numeric: tabular-nums`,统计大数字加重;系统字体栈不变(含 PingFang SC / Microsoft YaHei)。
- **主题机制**:`html[data-theme="dark"]` 显式暗色;`@media (prefers-color-scheme: dark)` + `html:not([data-theme])` 自动暗色(显式设置优先)。全站颜色只引用 token,禁止散落硬编码色值。

### 2. 三态主题切换(head.html 内联脚本 + 顶栏按钮)

- head.html 在样式表加载前内联 anti-FOUC 脚本:读 localStorage(`tu-theme`: light/dark/auto,缺省 auto)与系统偏好,落 `data-theme`(auto 时不落属性交媒体查询)。
- 顶栏加日/月/自动图标按钮(内联 SVG),点击轮换三态并持久化;按钮在 `server serve` 社区模式同样可用(主题是全局能力,不是本机模式专属)。

### 3. 本机面板品牌分支(模板条件化,不分叉代码)

- `head.html`:`{{if .Local}}` 分支品牌名(「Token 用量」)、页面 title 后缀、隐藏「排行榜」导航与注册/登录入口(本机免登录),副标注「本机数据 · 不出网」;`{{else}}` 保持「Token 排行榜」现状,社区部署行为零变化。
- `me.html` 页头、foot 同步分支文案。
- `.Local` 已存在于模板数据(web.go),无需动 Go。

### 4. Dashboard 模板微调(可读性/密度)

- statgrid 8 卡:主卡(当日消耗/成本)升权重、次卡收缩,标签-数值层级拉开;缓存命中率等百分比卡加进度环或色带(纯 CSS)。
- chartbox 标题与图表留白、坐标轴标签字号(经 ECharts textStyle)统一;devices 表格对齐与等宽。
- 刷新按钮改为 topbar 内的次级按钮样式(原生抽表单按钮)。

### 5. 图表主题统一(app.js)

- 调色板常量改为从 CSS token 运行时读取(`getComputedStyle(document.documentElement)`),与设计系统单一来源;柱图/折线/饼图配色、网格线、tooltip 底色随主题走。
- 主题切换时 dispose + 以新主题重建图表实例(option 构建函数化,数据来自 `#dash-data` 常驻 JSON,不重发请求)。

### 6. 文案修正(rebrand 残留)

- `ccusage report` → `token-usage report`、`ccusage.json` → 按现配置名词(`token-usage` 命名空间,ADR 0007)修正 settings.html 代码示例、about.html 说明、index.html 空状态,共 4 处文件。

### 7. 移动端与微动效

- 断点从单一 900px 扩为 ~640 / ~900 两档:statgrid 4→2→1、charts 纵排、topbar 导航换行不折叠(不引入汉堡菜单 JS)、rankcard 数字列窄屏下移。
- 克制动效:卡片/按钮 hover 过渡(150–200ms ease)、暗色切换时全局 `transition: color/background 0.2s`、rankcard hover 边框微亮;无滚动 reveal、无数字滚动。

## Constraints(红线)

1. **零外部请求**:不引任何 CDN 字体/图标/脚本;全部资源走现有 `/static` 内嵌。
2. **不动测试断言物**:元素 ID(`chart-hourly`/`dash-data`)、文案(「我的 Token」「刷新数据」「当日消耗」「缓存命中率」「连续活跃」「全员累计消耗」)、数字格式(Go 侧 `fmtTokens`/`fmtCost` 输出如 5.5K/1.2K)一律不变。
3. **server serve 零行为变化**:认证、上报 API、社区页面文案与结构不动(仅共享样式系统带来的视觉升级)。
4. **数据不出网**延伸到 UI 层:无遥测、无外链资源、离线可用。

## Implementation Surface

| 文件 | 动作 |
|------|------|
| `internal/server/web/static/style.css` | 全量重写(design tokens + 双主题 + 布局优化) |
| `internal/server/web/templates/head.html` | 品牌分支、主题切换按钮、anti-FOUC 内联脚本 |
| `internal/server/web/templates/me.html` | statgrid 层级、页头、刷新按钮归位 |
| `internal/server/web/templates/foot.html` | 品牌分支文案 |
| `internal/server/web/templates/index.html` / `about.html` / `settings.html` | ccusage 文案修正;index 结构微调 |
| `internal/server/web/static/app.js` | 图表主题函数化、token 化调色板、换肤重建 |
| `server/CONTEXT.md` | 已新增 Local Panel 词条 ✅ |

不新增 Go 代码改动(模板数据已备);`login.html`/`register.html`/`pricing.html` 仅被设计系统顺带覆盖,不单独优化。

## Acceptance Criteria

1. `token-usage web` 打开 `/me`:亮色精致、可切暗色,三态记忆跨刷新生效,切换无 FOUC、图表同步换肤。
2. 本机模式 topbar 为「Token 用量 · 本机数据 · 不出网」,无排行榜/登录/注册入口;`server serve` 页面品牌与功能与从前一致。
3. 375px 宽度(手机)无横向滚动,统计卡与图表可读。
4. 全站无任何外部网络请求(devtools network 面板仅 `/static/*` 与页面本身)。
5. `go test ./internal/server/... ./internal/e2e/...` 全绿(含 web_local e2e)。
6. 仓库内 `ccusage` 文案残留清零(`rg -i ccusage internal/server` 为空)。

## Ticket 拆分(已发布 `issues/`)

1. `01-design-tokens-dual-theme` — 双主题设计系统与三态切换(无阻塞;图表固定配色为已知中间态)
2. `02-local-panel-branding` — 本机面板品牌分支 + ccusage 文案修正(无硬阻塞,建议接 01 后)
3. `03-charts-theme-sync` — 图表调色板 token 化与换肤重建(← 01)
4. `04-dashboard-density` — /me 统计卡分层与可读性(← 01, 03)
5. `05-responsive-motion` — 断点体系与克制动效(← 04)
6. `06-verification` — 集成验收,spec 验收清单逐条勾验(← 01–05)
