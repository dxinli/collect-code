# 12: ChirpStack 桩（gRPC + MQTT 事件时序）

**What to build:** 一个能替代真实 ChirpStack 的桩：队列、发射确认、设备确认、上行事件都按官方语义发生，供无实物联调与 CI 使用。

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] 实现 gRPC Enqueue/GetQueue/FlushQueue 的桩语义：队列 FIFO、isPending 翻转、可查询
- [ ] 按 expected_uplink_interval_s 产生 uplink 事件（base64 data）到 MQTT integration Topic
- [ ] 队列项被取走时按配置产生 txack；confirmed 下行产生 ack
- [ ] 支持脚本化场景：设备离线/上线、队列满、延迟取走
- [ ] 桩行为与 ADR-0002 核实的官方语义一致，差异记录在测试说明
