# 05 — 渲染器补全:颜色 / 紧凑布局 / 宽度截断

**What to build:** SimpleTable 渲染的完整字节对等:ANSI 颜色码手写(表头蓝、ACTIVE 绿、百分比红);`--color/--no-color` 与 `NO_COLOR`/`FORCE_COLOR` 优先级;窄终端紧凑切换(usage <100 列、blocks <120 列)与 `--compact` 强制;CJK/emoji 显示宽度感知的对齐与截断;多行单元格;日期压缩。

**Blocked by:** 02 — tracer bullet。

**Status:** ready-for-agent

- [ ] `FORCE_COLOR` 下的报表输出 ANSI 码与参考实现字节一致
- [ ] 颜色开关优先级(`--color` > `FORCE_COLOR` > `--no-color` > `NO_COLOR` > TTY 探测)各分支 golden
- [ ] 宽终端阈值切换:固定 COLUMNS 下紧凑布局触发/不触发的 golden;`--compact` 强制紧凑
- [ ] CJK 与 emoji 内容的列宽计算、对齐、截断与参考一致(宽度表对齐到参考实现的 Unicode 版本)
- [ ] 多行单元格与日期压缩对等
