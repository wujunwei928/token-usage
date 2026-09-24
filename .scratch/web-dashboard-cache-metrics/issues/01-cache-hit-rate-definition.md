# 01 命中率口径与三比例分解 + 30 天趋势线

Status: resolved

## 事实

`internal/server/query.go` Dashboard():`CacheHitRate = read/(read+write)` 全历史;
`me.html` 用 `%.1f` 渲染 + 单段进度条。

## 改动

- 新增当日五计数器聚合(`TodayComposition` 或当日累加器);
  `CacheHitRate = todayRead/(todayRead+todayWrite+todayFresh)`,配 `HasRate bool`。
- 新增 `CacheSplit{Read,Write,Fresh}`(当日,和为 1,无数据为零值)。
- `DayPoint` 增 `HitRate float64` + `HasRate bool`(逐日同公式)。
- `me.html`:标签「当日缓存命中率」、`%.2f`、三段堆叠条替换单段 meter
  (保留 `class="meter"` / `class="meter-fill"` 类名锚定 branding_test;
  追加 `.m-write`/`.m-fresh` 修饰类)。
- `app.js` 近 30 天图加命中率折线(第三轴,百分比,无数据日断点)。

## 验收(TDD)

- Go:当日 seed(读 8900/写 100/新 1000)→ rate 0.89、split {0.89,0.01,0.10};
  昨日不同配比不影响当日值;昨日 DayPoint.HitRate 独立正确。
- HTML:含「当日缓存命中率」、meter-split 三段。
