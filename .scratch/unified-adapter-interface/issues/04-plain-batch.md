# 04 — plain 组迁移:gemini / kimi / openclaw / amp / copilot / goose / kilo

**What to build:** 其余七个通用 adapter 批次迁移(与 03 并行):默认 Sunday 起始、SummarizeByKey 键置换 session 分组、「一律先过滤再聚合」顺序走 profile 默认值;openclaw 的路径 flag 经工厂闭包注入。完成后全部 15 个通用 adapter 经新接口供能,旧注册路径仅剩 claude/codex 未迁。

**Blocked by:** 02 — 试点配方。

**Status:** done

- [x] 七个 adapter 的 spec 声明化、经接口 + profile 供能
- [x] 各自 golden 全绿(amp 4 / copilot 4 / goose 2 / kilo 2;gemini/kimi/openclaw 无 golden,以契约套件为防线)
- [x] copilot 空 rows 专属提示、amp/copilot 的 JSON 形态等展示层差异保持现状(归 CLI 层,不进 profile)
- [x] 定价加载语义逐 agent 原样保留在工厂闭包内
