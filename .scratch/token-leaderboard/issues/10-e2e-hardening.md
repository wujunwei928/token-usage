# 10 — 端到端收口:多 Agent fixture、embed 单二进制、部署说明

**What to build:** 把端到端覆盖补到完整形态:fixture 扩展到多 agent 混合(claude + codex + gemini 等)、5m/1h 缓存拆分、dedup 场景、跨小时/跨时区边界,断言 Tool 维度正确归位;前端静态资源与价格表随二进制 embed,验证"单文件部署"(一个二进制 + 一个 SQLite 文件路径即跑);补部署说明(环境变量、反代、数据备份)。全链路回归:report 命令 → 上报 → 榜单/仪表盘页面数字一致。

**Blocked by:** 05, 06, 07, 08, 09(全部功能就位后收口)。

**Status:** resolved

- [x] 多 agent fixture 上报后,榜单 Tool 筛选各工具数字与该工具单独统计一致
- [x] 5m/1h 拆分、dedup、跨小时边界的端到端断言通过
- [x] 无外部文件依赖:embed 后单二进制 + SQLite 即可起服务并渲染全部页面
- [x] 部署说明覆盖配置项、备份与升级价格表的方法
- [x] 全链路回归用例:同一份数据,CLI 本地报表、榜单、仪表盘三者口径一致

## Comments

- 2026-08-15 实现+验收完成。多 agent e2e(claude+codex+跨文件 dedup)、ECharts/样式/模板全 embed、server/pricing/model-prices.json(453 模型)、server/README.md 部署说明
