# 09 — `--install-timer` 定时上报与失败 UX 收口

**What to build:** `ccusage report --install-timer` 向当前用户 crontab 追加每小时执行一次 `ccusage report` 的条目(幂等:已存在则跳过并提示),安装/跳过结果有清晰输出;卸载途径(移除该行)一并提供。同时收口失败 UX:server 不可达、token 无效/被吊销、设备超 3 台被拒等场景,命令输出人类可读的原因与建议动作(如"去设置页重新生成 token"),退出码区分成功/配置错/网络错。

**Blocked by:** 02(命令已存在)。

**Status:** resolved

- [x] 安装后 crontab 含该条目;重复执行不产生重复行
- [x] 提供卸载;卸载后不再自动上报
- [x] token 失效场景的报错能指引用户去设置页
- [x] 成功/配置错/网络错三种退出码可区分,便于脚本使用
- [x] 测试覆盖安装幂等与主要失败场景的输出

## Comments

- 2026-08-15 实现+验收完成。timer.go 纯函数化 + install/uninstall 幂等单测;退出码 0/2/3/4 + 中文指引(e2e 断言)
