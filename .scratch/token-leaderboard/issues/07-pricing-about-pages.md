# 07 — `/pricing` 价格表页与 `/about` 数据说明页

**What to build:** 两个访客可看的信息页:`/pricing` 展示全部模型单价(input/output/cache read/cache write)与来源标注(official/estimated),文案说明计费规则(缓存写 5m/1h、分层、家族兜底),数据来自价格模块当前加载的价格表;`/about` 呈现数据说明与上榜规则(只读本地日志不传内容、Latest-wins、每人最多 3 台 Device、按天汇总、Anomaly Flag 条款),文案对齐截图第三、五页。

**Blocked by:** 04(价格模块与页面骨架)。

**Status:** resolved

- [x] `/pricing` 列出价格表全部条目,来源标注与价格模块一致
- [x] `/pricing` 与 `/about` 无需登录可访问
- [x] 计费规则文案与实际计算行为一致(分层、5m/1h、兜底)
- [x] 上榜规则文案与系统语义一致(Latest-wins、3 台上限、当日不上榜条款)

## Comments

- 2026-08-15 实现+验收完成。/pricing(453 模型 official/estimated)+ /about 数据说明与上榜规则;匿名可访问
