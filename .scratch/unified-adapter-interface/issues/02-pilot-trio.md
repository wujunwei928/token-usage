# 02 — 试点迁移三胞胎:droid / hermes / codebuff

**What to build:** 三个逐字节相同的 adapter 改由新接口供能,验证迁移配方:统一报表 spec 声明化(枚举翻译器与手写 load→detected→过滤→聚合管道消失,行为差异进 profile),all/ 的消费路径切入新注册表(RowSource 包装),adapter 本地报表入口保留薄门面转发共享管线(旧调用方继续编译)。其余 14 个 agent 仍走旧路径,仓库全程全绿。

**Blocked by:** 01 — expand 基座。

**Status:** done

- [x] 三胞胎的 spec 只剩声明(名字 + 工厂 + profile),管道代码为零
- [x] all-report 输出与迁移前逐字节一致(droid 6 / hermes 5 / codebuff 5 个 golden 全绿)
- [x] 契约测试套件覆盖三个 adapter
- [x] 旧 RegisterSpec 路径仍正常服务其余 adapter;Detected 语义不变
