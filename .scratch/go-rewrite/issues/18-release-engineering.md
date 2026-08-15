# 18 — 发布工程:GoReleaser / 快照更新 / 性能核对

**What to build:** 正式分发渠道:GoReleaser 多平台构建与版本注入(LDFLAG)、`go install` 可安装;内嵌定价快照的脚本化更新通道(拉上游三份数据、紧凑化、提交);与参考实现的性能核对(按文件大小均衡的并行读取与行预过滤生效,大 fixture 上不显著劣化);README(安装与用法)。

**Blocked by:** 16 — 全适配器就位;17 — CLI 表面定稿。

**Status:** ready-for-agent

- [ ] GoReleaser 配置产出多平台二进制,版本号注入 `--version` 输出
- [ ] `go install` 可安装并运行
- [ ] `make update-pricing`(或等价脚本)一键再生内嵌快照并保持 golden 全绿(定价变更不影响对等测试的机制就位)
- [ ] 性能核对:大 fixture 上与参考 Rust 版耗时不显著劣化(数量级内),并行读取与预过滤确认生效
- [ ] README 覆盖安装(各渠道)与全部命令用法
