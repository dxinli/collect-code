# 11: 事件上报：故障、超时、拒绝

**What to build:** 阀门故障、指令超时、设备拒绝这三类事实能主动上报成事件，且不重复刷屏。

**Blocked by:** 09

**Status:** ready-for-agent

- [ ] valve_fault 在故障码位图变化且非 0 时触发一次，恢复不重复触发
- [ ] cmd_timeout 在窗口耗尽时触发，reason=window_exhausted
- [ ] cmd_rejected 在设备侧拒绝时触发并携带设备错误码
- [ ] 事件按 event/{event_id} Topic 上报，dedup_key 相同不重复
