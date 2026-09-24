# token-usage:inclusive 输入口径适配层扣减(与参考实现的受控分叉)

ADR 0008 承诺 CLI 与 Rust v20 参考"语义对等"。但 zcode/codex 适配器把 OpenAI 系
inclusive 口径的 `inputTokens`/`prompt_tokens`(cached ⊂ prompt,铁证:
`totalTokens = inputTokens + outputTokens`)与 `cacheRead` 当互斥类分别入库,
导致缓存读双重计入:命中率被腰斩(用户实测 zcode 显示 49%,真实 ≈97%),
token 总量虚高一倍。经用户拍板(.scratch/web-dashboard-cache-metrics 票 06),
Token Usage 四元组的互斥语义(root CONTEXT.md)优先于参考实现行为:适配层用
`core.SubtractCachedOverlap` 扣减重叠(zcode、codex、gemini 三个适配器共用)。
代价:zcode/codex 报表数字与 Rust v20 参考不再字节级一致;golden 基线如需
对齐,应修参考实现而非回退本决策。
