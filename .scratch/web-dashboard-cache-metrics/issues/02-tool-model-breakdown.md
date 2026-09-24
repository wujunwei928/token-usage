# 02 工具×模型明细表 + 图表 tooltip 增强

Status: resolved

## 改动

- `query.go`:新增 `ToolModelRow{Tool,Model,Tokens,Cost,HitRate,HasRate}`(全历史,
  按 tokens 降序,封顶 50 行);`ByTool`/`ByModel` 从 `[]ModelSlice` 换成
  `[]NameStat{Name,Tokens,Cost,HitRate,HasRate}`(tokens 降序)。
- `me.html`:新 chartbox.wide「工具 × 模型明细」表(工具/模型/Token/成本/命中率,
  无缓存数据命中率显示 —);表格复用 .devices 风格 + .num 右对齐列。
- `app.js`:byTool/byModel 改读 `r.Name`;柱状图 tooltip 显示
  Token(fmtTokens)/成本/命中率;JSON 块同步新字段。

## 验收(TDD)

- Go:两工具两模型 seed → 行数、排序、每行 rate/cost 正确;
  无缓存模型 rate=0 且 HasRate=true;纯零分母 HasRate=false。
- HTML:表格表头齐全。
