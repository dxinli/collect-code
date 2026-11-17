# 13: LoRaWAN 上行接入

**What to build:** LoRa 节点的上行 payload 能按声明式位域解码成属性并上报，且断线重连不重复处理同一帧。

**Blocked by:** 12,05

**Status:** ready-for-agent

- [ ] 订阅 ChirpStack MQTT integration，按 DevEUI 映射到设备实例
- [ ] payload 按 profile 的声明式位域解码为点位；非法 payload 标 bad
- [ ] uplink 作为新鲜观测推进 last_success_at，并参与新鲜度计算
- [ ] 上行属性经同一物模型上报路径发出
- [ ] 断线重连后不重复处理同一 uplink（按事件 id 去重）
