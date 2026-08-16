# 03 — 高分歧组迁移:qwen / zcode / opencode / pi

**What to build:** 四个带行为旋钮的 adapter 按已验证配方迁移,profile 的每个旋钮被真实用例检验:Monday 周/月起始(zcode)、weekly Monday + monthly Sunday(opencode)、SessionAccumulator 分组与「先聚合按 lastActivity 过滤」顺序(qwen/zcode/opencode)、HasData 计入 Detected(qwen/zcode/opencode)、自定义路径 flag 经工厂闭包注入(pi 的多根路径)。完成后这四个 adapter 的 spec 声明化、本地枚举变门面。

**Blocked by:** 02 — 试点配方。

**Status:** done

- [x] 四个 adapter 全部经接口 + profile 供能,spec 只剩声明
- [x] Detected 现语义保持:数据源存在即算(opencode-detected-all golden 是锚点)
- [x] 周起始日 / session 分组 / 过滤顺序差异全部由 profile 表达,共享管线一份实现
- [x] 相关 golden 与契约套件全绿;pi 的路径 flag 经工厂闭包注入,接口无新字段
## Comments

**2026-08-16 评审勘误**:正文"opencode 用『先聚合按 lastActivity 过滤』"表述有误。opencode 的 profile 是 `SessionFilterAfter: false`(先按日期过滤再聚合),其 loader 在读取时即按窗口收窄——与 HEAD 行为一致,代码正确、issue 文字错误。
