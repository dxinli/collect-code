# 11: 事件总线与去重契约

**What to build:** 事件有唯一实例标识，重发不重复，状态翻转能产生新实例。

**Blocked by:** 02a

**Status:** ready-for-agent

- [ ] 事件实例键 `device:event:episode`；同实例重发复用同一 msg_id 与 dedup_key
- [ ] 状态翻转 episodes 递增；不会永久抑制第二次真实故障
- [ ] 订阅/分发/去重测试；`link_down/up` 与 `driver_error` 可由生产者投递
- [ ] 事件实例键构造规则写入文档
