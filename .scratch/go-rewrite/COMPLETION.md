# ccusage-go 完工记录

对齐目标:已安装的 ccusage 20.0.19(原生 Rust 二进制)。

## 最终验证结果

- `go test ./...`:9 个包全部通过;golden 套件 142 个用例,与参考二进制逐字节一致(stdout/stderr/exit code)。
- `scripts/compare.sh`(真实数据全矩阵,offline 模式):54 通过 / 4 失败,且 4 个失败全部源于**同一个已知偏差**(见下)。
- 性能:598 个真实 JSONL 上 `claude daily --offline`,Go 0.48s vs Rust 0.18s(同数量级)。

## 已知偏差(1 项)

**pi deepseek-v4-flash 2026-08-08 行的 1ULP 浮点差**:参考输出 `0.003975563200000001`,本实现 `0.0039755632`(第 16 位有效数字)。该行成本来自 pi 日志自带的 `cost.total` 分项求和,参考二进制使用了某种无法从仓库源码推出的求和顺序(仓库源码的时间排序会得到本实现的值;参考值确定可复现,但 96 种排列都可产生该字面值,未能定位其确切顺序)。仅影响全精度 JSON 输出中该单行;表格输出(两位小数)不可见。

## 对齐过程中发现并修复的关键事实

1. **仓库源码 ≠ 已发布二进制**:仓库 HEAD 领先于 20.0.19(内嵌定价快照、pi 排序、grok 命令、错误文案均有差异)。所有差异以二进制为准:
   - 三份内嵌定价快照(models-dev/litellm/fast-multiplier)直接从二进制提取(`scripts/update-pricing.sh`);
   - grok 命令在 20.0.19 不存在,已禁用注册(适配器代码保留);
   - 错误文案逐字对齐二进制探测结果。
2. **zsh 不分词陷阱**:早期 bash 探测中 `$args` 被 zsh 作为单 token 传递,产生误导性错误文案;golden.sh(python 参数列表)为权威。
3. `claude daily` 与 weekly/monthly/session 走两条不同的摄入管线(dedup 决胜序不同)。
4. JSON 输出键按字母序(serde Value 语义),仅 `--sections` 顶层保持插入序。
5. 空数据 totals 的 `totalCost: -0.0` 来自 Rust 空 f64 求和的 -0.0 起点。
6. pflag 的 NoOptDefVal 不消费空格取值,可选值旗标需在解析前归一化为 `--flag=value`(`internal/cli/normalize.go`)。
7. CCUSAGE_OFFLINE 环境变量在 20.0.19 已不被读取(仅旗标/配置)。

## 工单状态

01-18 全部完成。16 个适配器中 15 个随 20.0.19 二进制启用(grok 按上文禁用,代码就绪)。
