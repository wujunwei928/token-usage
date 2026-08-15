# CLI 行为级对等(含 legacy 兼容层)

Go 版 CLI 表面与 Rust 版做到行为级一致:旗标名、短旗标、别名、默认值、退出码、报错文案全部保留,包括看似冗余的 legacy 兼容层——`codex:daily` 冒号形式归一化、`-a` 在 blocks 命令上是 `--active` 而在其他命令报错、对已移除的 `--agent`/`--daily` 旗标给出迁移提示报错。仅 `--help` 布局允许使用 cobra 默认样式。

这些兼容层是有意保留的,不要在后续重构中"清理"它们:用户脚本和 shell 别名依赖这些行为,二进制可无缝互换。实现上通过 cobra 的 `Aliases`、自定义 `FlagErrorFunc`/`Args` 校验达到。
