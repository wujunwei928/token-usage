# 10 models.dev 同名模型固定取官方 provider 第一方价

Status: resolved

## 发现

验证票 09 时实测:core 的 models.dev 加载器按 Go map 随机顺序遍历 provider、
先到先得——同名 modelID 有 50+ 家条目(glm-5.3 转售价 0.4~1.79,还有
`zai-coding-plan`/`alibaba-token-plan` 等**包月渠道的 $0 计量条目**),
**每次重启随机摇一家**,本轮实测摇到过 $0(免费档)。没有用户 override
兜底时,live 层会间歇性把模型定成免费——正是票 08 修掉的错误类别。

## 定案(user)

记录模型官方渠道价格:每一个模型(家族)映射一个官方 provider,
同名条目固定取第一方价。

## 实现

- core 新增官方映射表(首 token → provider key,均经 models.dev 实数据
  验证存在):glm→zai、gpt/o*/codex→openai、claude→anthropic、
  gemini/gemma→google、grok→xai、deepseek→deepseek、qwen/qwq→alibaba、
  kimi→moonshotai、minimax→minimax、mistral 系→mistral、command→cohere、
  doubao→volcengine。
- loadModelsDevJSONMissing 重构:先按 modelID 收集全部候选(带 provider
  key),按 (官方? > 非零价? > 显式缓存价? > provider键字典序) 择一,
  消除 map 遍历随机性。$0 包月渠道因 provider 键 ≠ 官方键且"非零价"
  得分低而被天然排除。
- embedded models.dev 快照走同一加载器,一并受益。

## 验收

- 合成多 provider 文档:官方价恒胜(连跑 N 次稳定);无官方映射时
  取确定性赢家;$0 条目永不当选(除非唯一)。
- 真实 modelsdev.json(本地有):Find("glm-5.3") 恒为 1.4/4.4/0.26。
- core/server/e2e 全绿。
