# 13: LoRaWAN 上行接入

**What to build:** LoRa 节点的 payload 被正确解码成属性并上报，重连不重复处理。

**Blocked by:** 12b,05b,03b

**Status:** ready-for-agent

- [ ] 订阅 integration，DevEUI 映射到设备实例；未知 DevEUI 忽略并计数
- [ ] 位域解码：字节偏移、大端、有符号、长度、fPort 过滤；长度不足 → bad 且不推进 last_success_at
- [ ] 去重键：deduplicationId，缺失时 devEui+fCnt；内存窗口 1024
- [ ] uplink 作为新鲜观测进入 03b 的判定，并经 05b 上报
