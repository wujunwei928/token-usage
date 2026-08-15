# 字节级输出对等(自研表格渲染器)

表格与 JSON 输出在相同输入(fixture)、相同终端宽度、相同颜色设置下与 Rust 版字节一致,包括制表符边框、列对齐、ANSI 颜色码、CJK/emoji 宽度感知截断、以及窄终端下的紧凑布局切换。为此不使用现成 Go 表格库,而是移植 Rust `ccusage-terminal` crate 的 `SimpleTable` 渲染逻辑。

收益:Rust 二进制可直接对任意 fixture 生成 golden file,Go 版用字节级 diff 做回归测试,输出差异零歧义。JSON 输出形状与字段顺序同样固定。

## Consequences

- `internal/terminal` 包需要实现 display-width 计算(含 emoji/CJK 宽字符),不能依赖 `len([]rune)`。
- golden 测试必须固定终端宽度与 NO_COLOR 等环境,避免 TTY 探测引入的非确定性。
