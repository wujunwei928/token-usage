# 08 价目失真修复:glm-5.3 estimated 卡缓存价 $0 + 配置目录价目自动加载

Status: ready-for-agent

## 发现

用户质疑"当日成本 < $1"。实证:当日按 z.ai 第一方价应为 **$42.91**,显示
$0.10(差 400 倍)。因果:`glm-5.3` 不在内嵌快照 → family 回退剥到裸 `glm`
(跨代)拿到 **缓存读 $0** 的上古卡;用户用量 99% 是缓存读 → 全按 $0 计。
web 命令离线运行不联网拉价,`--pricing` 覆盖是唯一通道但无人知道要传。

models.dev 多源交叉的 z.ai 第一方价(每 1M):
glm-5.3: in $1.4 / out $4.4 / read $0.26;flash: $0.15 / $0.5 / $0.03;
GLM 惯例写 5m 免费、写 1h = 2×input(同 glm-4.7 官方卡)。

## 定案(user 三连推荐项)

1. 生成 `~/.config/token-usage/model-prices.json`(z.ai 第一方价)。
2. `--pricing` 未传时自动读 `<UserConfigDir>/token-usage/model-prices.json`
   (存在才读;web 与 server serve 同样生效)。
3. 通用防再犯:SourceFamily(estimated)卡的**零**缓存价按倍率推导
   (read 0.1×in、write5m 1.25×in;write1h 已有 2×in 惯例);
   成本卡用到估算价时加「估算」角标。

## 验收

- family 卡:read=0.1×in、w5m=1.25×in,source=estimated;官方/override 卡不受影响。
- ConfigPricingPath:文件存在/不存在两态。
- Dashboard:当日含 estimated 模型 → CostEstimated=true,/me 渲染「估算」角标。
- 单测 + e2e 绿;重跑今日成本 ≈ $40+(仅重启 web 即可,成本是渲染时算的)。
