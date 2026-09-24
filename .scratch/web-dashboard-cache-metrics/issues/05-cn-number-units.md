# 05 万/亿单位全站

Status: resolved

## 改动

- `web.go` FormatTokens:`≥1e8 → %.2f亿`、`≥1e4 → %.1f万`、以下原样。
- `app.js`:新增 JS 版 fmtTokens(同规则,<1万 千分位),应用于 hourly/daily/
  bar/pie 的 tooltip 与数值轴 formatter(USD 轴加 $ 前缀)。
- 测试断言迁移:server_test("5.5K"→"5500","1.5K"→"1500")、
  e2e_test 与 e2e/web_local_test 的全部 K 串按同规则改写。

## 验收(TDD)

- Go:FormatTokens 表驱动测试(0/9999/10000/12345678/123456789)。
- 全量测试绿(e2e 含)。
