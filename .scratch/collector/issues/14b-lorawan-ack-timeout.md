# 14b: C 级业务确认、窗口超时与重试

**What to build:** 「阀门真的动了」才算成功；等不到就明确超时，不谎报。

**Blocked by:** 14a,11

**Status:** ready-for-agent

- [ ] C 级算法：后续 uplink 的 opening 与 target 容差内且 state_word 非运行中 → completed
- [ ] deadline = enqueue_at + windows×expected_uplink_interval + margin；边界测试
- [ ] 重发至 cmd_retry_max；耗尽回 5042 并发 cmd_timeout(reason=window_exhausted)
- [ ] A+B 到但 opening 差 20% → 5041/device_mac，不误判 C
- [ ] 链路 down 期间暂停窗口计数
