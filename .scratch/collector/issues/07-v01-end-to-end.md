# 07: v0.1 端到端：485 传感器 → MQTT

**What to build:** 485 传感器数据经真实二进制采集并通过 MQTT 到达云端桩，断线与恢复行为正确，一键验收脚本给出结论。

**Blocked by:** 03,05,06

**Status:** ready-for-agent

- [ ] 真实二进制连模拟器，传感器点位按周期上报，云端桩逐字段收到正确值
- [ ] 连续假时钟快进 24h：上报条数符合预期、无意外 offline、无 bad/stale
- [ ] 拔线（停从站）后按 offline_after_intervals 产生 device_offline；恢复后发 online + 全量快照
- [ ] 同一总线多从站时，一台超时不阻塞其他设备采集
- [ ] 验收脚本可一键运行并输出通过/失败
