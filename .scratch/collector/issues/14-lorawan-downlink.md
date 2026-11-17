# 14: LoRaWAN 下行与分级确认

**What to build:** 云端下发的 LoRa 指令能经 ChirpStack 入队，并按「网关已发射/设备已收到/业务已完成」三档给出不同回执。

**Blocked by:** 13

**Status:** ready-for-agent

- [ ] 服务调用经 gRPC Enqueue 下发，queueItem 含 fPort/confirmed/data
- [ ] A 级（txack）与 B 级（ack）分别映射到 delivered，C 级业务 ACK 映射 completed
- [ ] 仅 A 级到 → 5041 且 data.stage=gateway_tx；A+B → 5041 且 stage=device_mac；C 到 → code=0
- [ ] 超时按 lora_ack_windows × expected_uplink_interval 计算，不按固定秒
- [ ] 链路 down 期间回 5040，不消耗窗口
