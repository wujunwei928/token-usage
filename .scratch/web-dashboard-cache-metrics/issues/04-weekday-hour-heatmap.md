# 04 工作时段热力图

Status: resolved

## 改动

- `query.go`:Dashboard 增 `Heatmap []HeatCell{Weekday(0=周一),Hour,Tokens}`,
  全历史 7×24 全格(含零),weekday 由 date 解析 `int(t.Weekday()+6)%7`。
- `me.html`:chartbox.wide「工作时段分布(星期 × 小时)」`#chart-heatmap`;
  JSON 块加 heatmap。
- `app.js`:ECharts heatmap,x 轴 0-23 时,y 轴周一..周日,visualMap 色带读
  CSS token,tooltip 用 fmtTokens;主题切换重建照旧。

## 验收(TDD)

- Go:seed 已知日期+小时 → 对应格子非零,其余为零,总数 168 格。
- HTML:含 chart-heatmap。
