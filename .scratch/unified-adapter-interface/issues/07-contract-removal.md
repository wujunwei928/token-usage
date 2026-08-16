# 07 — contract:旧形态全拆 + CLI 枚举机械替换

**What to build:** expand 阶段收官,删除全部旧形态:魔法下标注册表与旧 Spec 工厂、16 套本地 ReportKind 枚举与门面、15 个枚举翻译器、all/ 旧消费路径。CLI 各代命令文件对本地枚举的引用机械替换为共享 ReportKind——三代命令框架本身不动(收敛归候选 2,另行规划)。

**Blocked by:** 03、04、05 — 17 个 agent 全部经新路径供能,旧形态无调用方。

**Status:** done

- [x] 仓库内不再存在魔法下标注册、本地报表枚举、toXKind 翻译器、报表门面
- [x] CLI 三代框架结构原样,仅枚举类型统一为 core.ReportKind
- [x] golden 142 + 契约套件 + e2e 全绿
- [x] 删除量清点:spec 管道样板与 16 份报表拷贝消失,新增 adapter 的样板成本降到「一份声明」
## Comments

**2026-08-16 评审勘误与范围记录**:
1. 「仓库内不再存在报表门面」的准确实况:面向消费者的门面已全数删除;仅存两处非门面残留——grok 的 `SummarizeEntries` 转发(供 disabled 死路径编译,候选 2 清理)与 opencode 的 `summarizeEntries`(已改私有,其 ReportJSON 内部使用,非转发)。
2. spec「过渡策略」原定 CLI 仅做枚举机械替换、门面随候选 2 溶解;实际执行中门面成为死代码(共享管线已覆盖全部调用方),故提前删除并把 CLI run 闭包直接改调 `common.ReportRows`——超出原定范围但方向一致,已记录于 spec Further Notes。
3. 评审后续修复:周期 switch ×3 收敛至 `common.SummaryPeriod`;opencode 的 `openCodeMeta`/`rowsKey`/`periodKey` 残留翻译器改用 core 方法;`LoadRequest.Pricing` 死字段移除(ADR 0009 修订);`AdapterSpec` 恢复"每 spec 构造一次工厂"(定价每 run 加载一次,不再随 --sections 重复)。
