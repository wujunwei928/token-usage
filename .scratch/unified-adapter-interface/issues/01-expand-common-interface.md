# 01 — expand 基座:adapter/common 接口与共享管线

**What to build:** 仓库获得新的 Agent Adapter 契约层:adapter/common 定义 Adapter 接口(Agent / HasData / LoadEntries)、LoadRequest(Shared + Pricing)、LoadResult(Entries + Detected)、声明式 Report Profile 与共享报表聚合管线;core 出现唯一 ReportKind;注册表按 agent 名注册、配单一显式有序 roster,与既有魔法下标注册**并存**(expand 阶段,不拆旧)。契约测试套件落地,并在两个真实 adapter 上跑绿——一个 JSONL 扫描型(droid)、一个 SQLite 型(zcode,复用其既有 fixture 模式)。本工单不切换任何消费者,一切行为零变化。

**Blocked by:** None — can start immediately.

**Status:** done

- [x] Adapter / LoadRequest / LoadResult / Report Profile / 共享聚合管线在 adapter/common 可用,形状与 ADR 0009 一致
- [x] core.ReportKind 四值单一枚举就位(KindDaily/KindWeekly/KindMonthly/KindSession)
- [x] 按名注册表 + 显式有序 roster 可用,与旧 RegisterSpec 并存互不干扰
- [x] 契约测试套件(条目按时间有序、Dedup Hash 唯一、四种 kind 聚合确定性、profile 旋钮生效)对 droid 与 zcode 跑绿
- [x] golden 142 用例与全部既有测试零变化
