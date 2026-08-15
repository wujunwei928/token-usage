# 07 — session 报表 + 维度拆分

**What to build:** 按 `(Project, Session)` 聚合的 session 报表与 `--id` 单 session 明细;`--breakdown` 表格行按模型拆分(Model Breakdown);`--instances` daily 按项目分组;`--project <name>` 过滤与 `--project-aliases a=X,b=Y` 重命名。全部含表格与 `--json` 两种形态。

**Blocked by:** 06 — 复用其过滤与 JSON 输出基建。

**Status:** ready-for-agent

- [ ] session 报表按 `(Project, Session)` 聚合 golden
- [ ] `--id <sessionId>` 输出单 session 明细
- [ ] `--breakdown` 在 daily/weekly/monthly/session 上按模型拆行
- [ ] `--instances` daily 行按 Project 分组
- [ ] `--project` 过滤生效;`--project-aliases` 重命名出现在行与 JSON 中
- [ ] 以上全部旗标的 `--json` 形态 golden 字节一致
