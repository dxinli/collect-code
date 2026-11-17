# 16b: 恢复补发与去重

**What to build:** 恢复后关键事件按序补齐，重复补发不产生重复逻辑事件。

**Blocked by:** 16a

**Status:** ready-for-agent

- [ ] 恢复后先补发事件（时间序），再发每设备最新快照
- [ ] 同一缓存事件重发复用稳定 msg_id+dedup_key；断线两次只产生一个逻辑事件
- [ ] 断线期间属性 quality 正确（stale），快照 ts 不前移
- [ ] 重启不阻塞启动；缓存不跨重启承诺写入文档
