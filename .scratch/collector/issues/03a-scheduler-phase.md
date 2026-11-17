# 03a: 调度相位与单设备 deadline

**What to build:** 假时钟快进 24 小时，tick 时刻不漂移、超时设备不拖死总线。

**Blocked by:** 02a

**Status:** ready-for-agent

- [ ] 假时钟推进 24 h（60 s 周期）得到 1440 个 tick，相邻计划间隔恒为 period
- [ ] `next = start + k*period` 不累积；missed_tick 计数与日志字段可断言
- [ ] 单设备 read_deadline 到期：该设备点位标 bad，立即处理下一台
- [ ] 循环真实使用注入的 Clock（FakeClock 驱动），无 `time.Sleep`
