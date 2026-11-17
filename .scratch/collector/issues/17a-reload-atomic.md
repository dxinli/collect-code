# 17a: 配置重载：解析与原子交换

**What to build:** 坏配置不生效、好配置原子切换，不留半配置状态。

**Blocked by:** 01b,03a

**Status:** ready-for-agent

- [ ] 解析失败整体不应用并记 config_rejected，旧配置继续运行
- [ ] 原子交换：并发 SIGHUP 与 tick 无撕裂；每设备恰好一个 poller
- [ ] 热重载矩阵中「是」的块（devices/profiles/timing）生效
- [ ] 重载造成的 missed_tick 按 03a 规则记录，不额外丢 tick
