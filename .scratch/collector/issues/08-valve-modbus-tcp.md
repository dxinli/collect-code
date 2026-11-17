# 08: Modbus TCP 阀门读取与上报

**What to build:** 网关上的一台电动阀能被读取开度/目标/状态/故障并上报。

**Blocked by:** 04b,05b,03b

**Status:** ready-for-agent

- [ ] TCP 链路读取阀门 profile 的全部点位并上报
- [ ] 覆盖非默认 profile（不同地址/scale/类型）
- [ ] 阀门离线/恢复更新在线状态（事件由 11 的 eventbus 承载）
- [ ] 写操作不在本 ticket（归 09）
