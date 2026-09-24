# 02: 本机面板品牌分支与 ccusage 文案修正

**What to build:** `token-usage web` 打开即是「Token 用量」品牌:顶栏品牌名、页面 title、页头与页脚均按本机模式分支,副标注「本机数据 · 不出网」;顶栏不再出现排行榜、登录、注册入口(本机免登录)。`server serve` 社区模式的「Token 排行榜」品牌、导航与功能保持不变。同时清理模板中 rebrand 前的 `ccusage` 残留文案(设置页代码示例、数据说明、排行榜空状态),统一为 `token-usage` 命令与现配置名词。

**Blocked by:** None (can start immediately;建议接在 01 之后做,两者同触顶栏,按序可避免改动交叠)

**Status:** ready-for-agent

- [ ] web 模式:品牌、title、页头、页脚为「Token 用量 · 本机数据 · 不出网」,导航无排行榜/登录/注册
- [ ] serve 模式:社区页面品牌与结构与从前一致(仅承袭 01 的视觉升级)
- [ ] 模板内 `ccusage` 字样清零,设置页示例命令与现配置名词一致、可照抄运行
- [ ] e2e 断言文本(「我的 Token」「刷新数据」)未被改动,`go test ./internal/server/... ./internal/e2e/...` 全绿
