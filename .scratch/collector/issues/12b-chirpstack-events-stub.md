# 12b: ChirpStack 桩：上行与确认时序

**What to build:** 桩按真实时序产生上行/发射确认/设备确认，供 LoRa 链路联调。

**Blocked by:** 12a,06b

**Status:** ready-for-agent

- [ ] MQTT integration fixture：topic `application/{app}/device/{devEui}/event/{up|txack|ack}`，data 为 base64
- [ ] Enqueue 只入队；下一次模拟上行取队头，isPending=false 并发 txack
- [ ] 仅 confirmed 下行在后续窗口发 ack
- [ ] 场景控制：unit offline、queue_full、delayed take
- [ ] 与官方语义的差异用 expected-diff 文件显式 xfail
