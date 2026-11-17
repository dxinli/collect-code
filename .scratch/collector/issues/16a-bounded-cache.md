# 16a: 北向断线有界缓存

**What to build:** 断网不被拖垮：有界、可观测丢弃、不写满磁盘。

**Blocked by:** 05b,03b

**Status:** ready-for-agent

- [ ] 事件队列 1000×≤2KB；预留 1 槽保证 event_dropped 自身不被丢
- [ ] 属性仅保留每设备最新快照（覆盖写，不排队）
- [ ] 满时丢最旧并聚合 dropped_count，恢复后合并上报
- [ ] 存储占用有上限断言（预算内）
