# 配置使用独立的 token-usage 命名空间,不与上游 ccusage 共享

ccusage-go 此前与上游 ccusage 一样,从 `./.ccusage/ccusage.json` 与 Claude 配置目录(`~/.config/claude`、`~/.claude`)发现 ccusage.json。两个实现共享同一批配置文件意味着任何一方的格式演进(新键、新分节、语义变化)都可能让对方的行为漂移甚至出错——用户明确要求隔离("防止后续不兼容"),命名为 `token-usage`。

决策:配置发现改为 ccusage-go 自己的命名空间,发现顺序 `./.token-usage/config.json`(工作目录)→ `TOKEN_USAGE_CONFIG_DIR`(逗号分隔,整体替换全局查找)→ `~/.config/token-usage/config.json` → `~/.token-usage/config.json`。文件格式(defaults/commands 分节、pricingOverrides 等)不变,`--config` 显式指定任意路径也不变。`CLAUDE_CONFIG_DIR` 从此只影响 claude 数据适配器(projects JSONL 目录),不再参与配置发现。排行榜 Device 文件随之迁到 `<UserConfigDir>/token-usage/device.json`,首次加载时从旧位置 `<UserConfigDir>/ccusage/device.json` 自动迁移,既有设备身份不受影响(3 台上限内不重复占用)。

## Consequences

- 与上游 ccusage 的行为分歧再 +1:上游用户的存量配置不会被自动发现,需复制到 `~/.config/token-usage/config.json`(内容不用改)。
- golden/e2e 的密封环境(HOME 指向 fixture、env 从零构造)不受宿主配置影响;测试注入配置走 `--config` 或 `TOKEN_USAGE_CONFIG_DIR`。
- `docs/config-schema.json` 描述文件结构,与路径解耦,无需变更。
- 今后所有 ccusage-go 独有的配置键(如已规划的能力)都落在这个命名空间内,与上游的键空间互不污染。
