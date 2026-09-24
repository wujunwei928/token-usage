# 03 缓存净节省 + 回本次数卡

Status: resolved

## 改动

- `pricing.go`:新增 `CacheSavingsForHourRow(r *HourRow) float64`:
  `(read×(Input−CacheRead) − 5m×(CacheWrite5m−Input) − 1h×(CacheWrite1h−Input))/1e6`,
  未知模型贡献 0。
- `query.go`:Dashboard 增 `CacheSavings float64`(当日逐行求和)与
  `ReadPerWrite float64`(当日读÷写,写为 0 时零值)。
- `me.html`:新卡「当日缓存净节省」fmtCost + 副行「回本 N 次/写」(0 时显示 —);
  与此同时合并「使用工具」「使用模型」为「工具 × 模型」`N × M` 一卡。

## 验收(TDD)

- Go:对照 resolve() 出的卡价(claude-sonnet-4-5)手算期望值断言;
  Dashboard 当日净节省 = 各行之和;昨日行不计入。
- HTML:含「当日缓存净节省」与「工具 × 模型」。
