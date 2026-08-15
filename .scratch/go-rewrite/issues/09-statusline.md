# 09 — statusline:状态栏单行输出

**What to build:** `ccusage statusline` 从 stdin 读 Claude Code hook JSON(session_id、transcript_path、model、cost.total_cost_usd、context_window、effort),输出单行状态:模型(含 effort)、💰 会话成本/当日成本/Block 余额(Nh Nm left)、🔥 Burn 指示器(🟢/⚠️/🚨,阈值 2000/5000 非 cache token/min)、🧠 上下文占用百分比(50/80 阈值着色)。含 `${TMPDIR}/ccusage-semaphore/<session>.lock` 缓存(transcript mtime + `--refresh-interval` 键控、活动进程信号量去重)与 usage-limit reset 提示。旗标:`--visual-burn-rate off|emoji|text|emoji-text`、`--cost-source auto|ccusage|cc|both`、`--cache/--no-cache`、`--refresh-interval`(默认 1)、`--context-low/medium-threshold`(默认 50/80)、`--offline`。

**Blocked by:** 08 — 依赖 Block 识别与 Burn Rate。

**Status:** ready-for-agent

- [ ] 参考实现的 statusline hook fixture 从 stdin 注入,输出单行字节一致
- [ ] 会话成本、当日成本、active Block 余额段数值与格式对齐
- [ ] Burn 指示器三档判定与 `--visual-burn-rate` 四模式 golden
- [ ] 上下文百分比 50/80 着色与阈值可调
- [ ] 信号量锁缓存:刷新间隔内命中缓存不重算;并发活动进程去重;`--cache/--no-cache` 行为对等
- [ ] `--cost-source` 四取值行为对齐
- [ ] usage-limit reset 时间从报错日志解析并显示
