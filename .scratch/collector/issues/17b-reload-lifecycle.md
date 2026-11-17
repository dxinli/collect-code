# 17b: 配置重载：设备与服务生命周期

**What to build:** 增删设备、改周期时有明确收尾，在途调用不被腰斩。

**Blocked by:** 17a,09,10,16b

**Status:** ready-for-agent

- [ ] 新增设备启动、移除设备停止、发一次 device_removed
- [ ] 在途服务调用按旧快照走完；device_removed 与调用终态的顺序明确并测试
- [ ] 改采集周期在下一次 tick 生效
- [ ] 断线缓存保留；被移除设备的事件仍补发
