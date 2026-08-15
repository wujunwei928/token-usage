# 03 — 账号体系与 User Token 网页管理

**What to build:** 自建轻账号:注册(昵称、密码、城市、头像——首期预置头像/首字母)、登录(密码 bcrypt 哈希、cookie 会话)、资料编辑。登录后设置页可生成与吊销 User Token(页面仅展示一次,库存哈希),供客户端配置。`/v1/report` 继续只认 tokens 表(与 01 的种子 CLI 并存,种子保留给测试/运维)。页面风格与现有 SSR 路线一致(html/template,无 Node 构建链)。

**Blocked by:** 01(users/tokens 表已建)。

**Status:** resolved

- [x] 注册→登录→登出全流程;密码以 bcrypt 哈希存储
- [x] 会话走 cookie;`/me` 类页面未登录重定向到登录页
- [x] 设置页生成 token 仅明文展示一次;吊销后该 token 上报被拒
- [x] 资料可编辑城市与头像;城市进入 users 表供榜单筛选
- [x] 端到端测试:网页注册→拿 token→用该 token 成功上报→吊销→再报 401

## Comments

- 2026-08-15 实现+验收完成。web.go 账号流 + login/register/settings 模板;TestAccountFlowAndDashboard:注册→签发 token→上报→吊销全链路
