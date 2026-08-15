# 01 — Golden 基建 + 仓库脚手架 + `--version`

**What to build:** 仓库成为可构建的 Go module,产出 `ccusage` 二进制,`-v/-V/--version` 输出与参考 Rust 实现字节一致;对等验证机制端到端跑通——fixture 从参考实现迁入、golden 再生脚本、CI 内纯 Go 字节 diff 测试。这是后续所有工单的验证地基,本工单自身用 `--version` 的 golden 验证机制可用。

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] `go build` 产出二进制,cobra root 命令骨架就位(单 module,包布局按 ADR-0004)
- [ ] `-v`、`-V`、`--version` 三种形式输出与参考实现字节一致,以 golden 测试验证
- [ ] 参考实现的 claude/codex/statusline 测试 fixture 迁入本仓 testdata
- [ ] golden 再生脚本就位:在有 Rust 构建的机器上一键再生,固定 TZ、NO_COLOR/FORCE_COLOR、COLUMNS、CLAUDE_CONFIG_DIR 等环境,消除 TTY 探测非确定性
- [ ] CI(无 Rust 工具链)跑 golden 字节 diff 通过
- [ ] `go vet` 与静态检查通过
