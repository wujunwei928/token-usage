# 04 — 数据正确性:Dedup Hash / sidechain / advisor iteration

**What to build:** 摄入管线的数据正确性收口。交付:按 Dedup Hash(`(message.id, requestId)`)去重,sidechain 重放进 `(message.id, 空)` 容错桶,同键替换序为 非 sidechain > Token Usage 总量大 > 带 speed;advisor iteration 子条目按各自模型拆分计量;`<synthetic>` 模型、无 semver version 行拒绝、嵌套 progress 行(`data.message.message.usage`)计入等边角规则。

**Blocked by:** 02 — tracer bullet。

**Status:** ready-for-agent

- [ ] 去重替换序三项规则的表驱动单测,测试值移植自参考实现入口 crate 的内嵌测试
- [ ] sidechain 重放进入 `(message.id, 空)` 容错桶的组合场景 golden
- [ ] advisor iteration 子条目按各自模型拆分,出现在 Model Breakdown 中
- [ ] `<synthetic>` 模型条目行为与参考一致
- [ ] 无 semver version 的行被拒绝;嵌套 progress 行的 usage 被计入
- [ ] 以上全部边角的组合 fixture 端到端 golden 字节一致
