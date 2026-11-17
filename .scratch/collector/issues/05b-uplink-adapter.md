# 05b: 上行适配器与下行订阅

**What to build:** 属性与事件能按契约发出去，下行指令能被解析成服务调用。

**Blocked by:** 05a,03b

**Status:** ready-for-agent

- [ ] property snapshot/delta 报文与 golden JSON 逐字段一致
- [ ] delta 去抖：首次变化起窗，窗口结束发；连续变化每 delta_debounce_ms 至多一条
- [ ] EventReport 可发布（v0.1 的离线/恢复事件出口）
- [ ] 订阅 service/invoke 并解析为 Invocation；非法报文拒绝且不 panic
- [ ] quality/ts 原样序列化，不被 Adapter 改写
