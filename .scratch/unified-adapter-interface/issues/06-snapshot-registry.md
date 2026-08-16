# 06 — snapshot 接口化:排行榜客户端经注册表遍历

**What to build:** 排行榜客户端 snapshot 的 17 个直接加载调用改为注册表遍历——新增 agent 自动进入上报,无需改 snapshot。claude 的对账专用 daily 管线与 codex 的有损事件桥按现状保留并注明缘由(排行榜只需小时格,损益已接受)。

**Blocked by:** 03、04 — 全部 15 个通用 adapter 经接口供能(claude/codex 走保留路径,故不等 05)。

**Status:** done

- [x] snapshot 经注册表遍历获取各 agent 条目,日期窗口过滤行为不变
- [x] e2e 排行榜全链路绿(当日聚合、Latest-wins 覆盖、多 agent 去重、backfill)
- [x] claude 对账管线与 codex 事件桥保留,注释说明有损边界
- [x] 上报快照内容与迁移前一致(对账用例验证)
