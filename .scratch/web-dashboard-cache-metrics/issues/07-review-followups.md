# 07 Dashboard 聚合器结构重构(评审遗留)

Status: needs-triage

双轴 code-review(b9aa742)通过但留下的 P2 判断项,非缺陷,择机处理:

- **Divergent Change**:`Dashboard()` 单循环 ~120 行喂 8 个维度,后续每个新指标
  都改同一块。按维度拆分聚合器。
- **Primitive Obsession**:`(HitRate float64, HasRate bool)` 成对出现在
  DashboardData/NameStat/ToolModelRow/DayPoint,可收成小 `Rate` 类型。
- **双胞胎漂移**:JS `fmtTokens` 与 Go `FormatTokens` 跨模板边界必然成对,
  目前只有 Go 侧被测试锚定;可在 e2e 里加一条对 /me 渲染数字的锚定。
- **golden 存量漂移**:pi/omp/jq 7 个 case 在基线 HEAD~1 同样失败
  (`[pi]` 前缀、模型名截断差异),与本仓库近期改动无关,待与参考实现对账。

依据:.scratch/web-dashboard-cache-metrics/ 评审工件(diff.txt)。
