# 09 web/server 启动时自动拉取 models.dev 实时价目

Status: resolved

## 背景

票 08 修完价目失真后,用户追问:后续调价还要手动改 model-prices.json 吗?
用户拍板:不新增独立命令,在 web / 排行榜子命令**启动时拉取一次**(独立命令
容易忘)。附带确认:成本是渲染时现算的,价目一变**全部历史立即重算**,
无需重灌。

## 实现(`4add750` 后续提交)

- `core.LiveModelsDevPricing()` 导出既有惰性 live models.dev 管线(带 60s
  失败退避),server 不再自建 HTTP 逻辑。
- `PricingTable.EnableLiveModelsDev(warm)`:warm=true 启动即取,成功则后续
  查表全走进程内缓存;**失败则禁用该层**——请求路径永不触网、永不卡顿。
- `resolve()` 分层:override > 内嵌快照 > **live models.dev** > family 估算。
  新增 `SourceLive`(非 estimated,不加角标);/pricing 页来源标签与文案补 live。
- `web`/`server serve` 加 `--offline` 旗标跳过拉取;e2e 启动 web 传 --offline,
  防无网沙箱每次 +10s。

## 用户侧语义

- 你配置文件里的 glm-5.3/glm-5.3-flash **恒最高优先**,自动同步永远不覆盖。
- 新模型:内嵌快照没有 → live models.dev 有真价(启动可联网时)→ 都没有才
  family 估算 +「估算」角标。基本告别手动同步。
- z.ai 调价:重启 web 即生效(模型在 models.dev 有第一方价时)。

验证:server/cli/core/e2e 全绿;真机冒烟输出「models.dev 实时价目已加载」。
