# 17: SIGHUP 差别重载

**What to build:** 现场改配置后 SIGHUP 即生效，坏配置整体不应用且服务继续运行，移除设备有明确收尾。

**Blocked by:** 01,03

**Status:** ready-for-agent

- [ ] 改 devices/profiles 后 SIGHUP：新增设备启动、移除设备停止并上报一次 device_removed
- [ ] 改周期立即生效；未完成服务调用按原参数走完；断线缓存保留
- [ ] 解析失败整体不应用并记 config_rejected，旧配置继续运行
- [ ] 重载期间不丢 tick、不产生半配置状态
