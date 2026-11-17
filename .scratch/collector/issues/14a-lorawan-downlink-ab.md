# 14a: LoRaWAN 下行与 A/B 级确认

**What to build:** 指令经 ChirpStack 入队，能区分「已发射」与「设备已收到」。

**Blocked by:** 13,09,10

**Status:** ready-for-agent

- [ ] Enqueue 的 queueItem 字段（devEui/fPort/confirmed/data/object.ref）正确
- [ ] txack → delivered（stage=gateway_tx）；ack → delivered（stage=device_mac）
- [ ] 仅 A 级 → 5041/gateway_tx；A+B → 5041/device_mac；两者本身永不回 0
- [ ] 链路 down → 5040，且不消耗回执窗口
