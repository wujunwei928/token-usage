# 16 — 混合格式适配器批 B:amp / copilot / gemini / kimi / qwen / openclaw

**What to build:** 六个非标准格式或特殊路径的 Agent Adapter:amp(JSON threads 而非 JSONL)、copilot(OTel 导出文件)、gemini、kimi、qwen、openclaw(`--open-claw-path` 自定义路径)。完成后 16 个适配器齐全,All-Report 全量可用。

**Blocked by:** 12 — 适配器框架。

**Status:** ready-for-agent

- [ ] amp JSON threads、copilot OTel 导出在适配器内转换为统一 Usage Entry,映射正确
- [ ] 六个适配器各自的目录/格式发现解析对等
- [ ] 每适配器 golden(表 + `--json`)
- [ ] `--open-claw-path` 生效并有 golden
- [ ] 16 agent 齐全后的 All-Report 合并报表 golden(`--sections`、`--by-agent` 全量形态)
