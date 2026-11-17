# 15: 下行队列治理

**What to build:** 设备离线期间连续下发同一目标时，设备上线只执行最新值，旧目标不会再被执行，且丢弃行为可观测。

**Blocked by:** 14

**Status:** ready-for-agent

- [ ] 同设备同 service 始终只保留最新一条（合并是常态，不是超限才做）
- [ ] 每设备 pending 超过 4 时淘汰最旧并记 cmd_dropped(reason=queue_full)
- [ ] 合并用 GetQueue 精确撤销，不调用 FlushQueue
- [ ] 若旧目标已被取走/下发，记 cmd_superseded_after_send，随后以当前目标纠正而非重放
- [ ] 运维子命令 queue flush --device 需二次确认
