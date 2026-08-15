# ADR 0001: 客户端只上报聚合数据,不上传原始用量条目

## Status

Accepted(2026-08-15)

## Context

Token 排行榜需要采集用户的 AI 编程 agent 用量。数据源头是本地 JSONL 会话日志,其中除了 token 计数,还包含会话内容、项目路径等敏感信息。可选方案:

1. 上报原始 Usage Entry,服务端聚合;
2. 客户端聚合为 (device, date, hour, tool, model) 五计数后上报;
3. 客户端算好成本一并上报。

## Decision

采用方案 2:客户端在本地完成解析、去重、按小时×工具×模型聚合,仅上报五类 token 计数(input / output / cache read / cache write 5m / cache write 1h);成本由服务端按统一价格表折算(见系统设计文档 Q7)。

## Consequences

- 原始条目永不离开用户设备,"不统计代码或对话内容"的产品承诺由架构保证,而非靠服务端自律。
- 服务端日后无法回溯比小时×模型更细的维度;若未来需要更细粒度,只能扩大上报粒度且历史数据不可补。
- 价格口径修正只需改服务端价格表,历史成本可随时重算。
- 聚合逻辑必须与 ccusage-go CLI 报表口径一致(复用同一套 adapter/dedup),否则榜单与本地 `ccusage` 输出对不上。
