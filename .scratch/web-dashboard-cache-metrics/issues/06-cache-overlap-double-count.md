# 06 zcode/codex 缓存重叠双重计入修复

Status: ready-for-agent

## 发现

GLM OpenAI 兼容端点的 `inputTokens`(codex 同:`prompt_tokens`)是**含缓存的
全量 prompt**(OpenAI 语义 cached ⊂ prompt)。zcode/codex 适配器把 input 与
cacheRead 当互斥类分别入库 → 缓存读双计。

证据(用户 zcode `raw_usage_json`):`totalTokens = inputTokens + outputTokens`
分毫不差;每请求 `input ≈ read + 1~3k`(真·新增)。结果:zcode 命中率 49%
(真实 ≈97%),当日 token 总量虚高一倍。

## 定案

User 拍板:修 zcode+codex。gemini 适配器 `subtractCachedOverlapTokens`
(parser.go:395)是仓库内现成先例。

## 改动

- zcode loader(DB 与 JSONL 两条路径):`Input = input - min(input, read+write)`(clamp)。
- codex(snapshot.go codexEntries):`Input = input - min(input, cached)`。
- 测试:zcode loader_test 与 codex 相关测试的 fixture 改为断言扣减后语义(TDD 先红后绿)。
- 已知代价:CLI zcode/codex 报表数字与 Rust v20 参考不再字节级一致(参考实现疑似同样双计);
  web.db 历史行需重灌:`token-usage web --since <起始日>`。

## 验收

- 单测:input 含重叠时,入库 Input=差值、CacheRead 不变;read>input 时 clamp 到 0。
- 全量测试绿。
