# 品牌独立:统一更名为 token-usage

仓库此前叫 ccusage-go——名字表达的是"对标上游 ccusage 的 Go 版"。随着配置迁入独立的 token-usage 命名空间(ADR 0007)、zcode 等超越上游的适配器落地(ADR 0006),工具已经是自有产品而非上游镜像,三层命名(仓库 ccusage-go / 命令 ccusage / 配置 token-usage)反而成为混乱源,且发布二进制 `ccusage` 与用户已安装的上游 npm 版在 PATH 上直接冲突。用户决定:统一都叫 `token-usage`。

决策内容:

- **Go module**:`github.com/wujunwei928/token-usage`;仓库目录同步更名。
- **命令与输出**:二进制 `token-usage`(`cmd/token-usage`);版本行 `token-usage 0.1.0`(放弃跟踪上游版本号,启用自有版本线);帮助/错误提示、statusline 的 cost 标签(`cc / $X token-usage`)与状态栏锁目录一并更名。与上游从"字节级对等"正式转为"语义对等":命令面、语义、退出码跟随参考,品牌字符串刻意不同。入库的 golden 文件从此是自有规格,从参考二进制再生成后需重放品牌字符串调整。
- **环境变量**:`TOKEN_USAGE_*` 为主、旧名兜底的双读过渡——`TOKEN_USAGE_REPORT_SERVER/TOKEN`(兜底 `CCUSAGE_REPORT_*`)、`TOKEN_USAGE_DEVICE_FILE`(兜底 `CCUSAGE_DEVICE_FILE`)、`TOKEN_USAGE_MODEL_ALIASES`(兜底 `CCUSAGE_MODEL_ALIASES`)。statusline 的 costSource 输入值接受 `token-usage`,旧值 `ccusage` 继续可用。
- **上游指代保留**:对 ryoppippi/ccusage 的出处引用、`ccusage-core`/`ccusage-terminal`/`ccusage-adapter-*` 等 Rust crate 名(移植出处)、脚本中指向上游参考二进制的路径,均保留 ccusage 字样。

## Consequences

- golden 期望输出中的品牌字符串(命令提示、版本行、statusline 标签)随更名一次性重写;此后 golden 的权威来源是本仓库,`scripts/golden.sh` 从参考再生成的产物需人工重放品牌调整。
- 已部署的旧环境变量、旧 costSource 值 `ccusage`、旧 crontab 条目(记录的是绝对路径)继续工作;新文档只宣传 `TOKEN_USAGE_*`。
- PATH 冲突消解:`token-usage` 与已安装的上游 `ccusage` 可共存,`scripts/compare.sh` 的双实现差分因此更可用。
- ADR 0001–0004 建立的"字节级对等"目标由本 ADR 收束为历史阶段;后续与上游的同步以语义为准。
