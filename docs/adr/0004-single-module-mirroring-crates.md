# 单 Go module 镜像 Rust crate 结构

仓库采用单一 Go module,包边界与参考实现的 Rust crate 一一对应:`cmd/ccusage`(≈crate ccusage)、`internal/core`(≈ccusage-core)、`internal/cli`(≈ccusage-cli)、`internal/config`(≈ccusage-config)、`internal/terminal`(≈ccusage-terminal)、`internal/adapter/<agent>`(≈rust/adapters/*)。刻意不用 Go 社区的 pkg/ 惯例布局,换取与参考实现逐文件对照的移植定位能力——移植期任何行为疑问都能直接跳到对应 crate 求证。
