# 15a: 下行队列合并与精确撤销

**What to build:** 设备离线时队列始终只留最新目标，且不误删别人的条目。

**Blocked by:** 14a,12a

**Status:** ready-for-agent

- [ ] 同 device+service 始终只留最新（合并是常态）
- [ ] GetQueue 精确撤销；RPC 日志断言 FlushQueue 调用数=0
- [ ] foreign ref 条目与 isPending=false 条目不被删除
- [ ] 确定性竞态：GetQueue 后队头被取走 → cmd_superseded_after_send，随后 Enqueue 当前目标
- [ ] 被取代且确实撤销时发 cmd_dropped(reason=superseded)，预期条数明确
