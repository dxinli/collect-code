# 04b: Modbus 读批、质量与链路状态

**What to build:** 一台 Modbus 设备能按 profile 批量读取，失败被正确归到点质量或链路状态。

**Blocked by:** 04a,03b

**Status:** ready-for-agent

- [ ] 按 profile 批量读、类型/缩放/字节序正确；覆盖非默认站号与负温度
- [ ] 映射表：CRC/短帧/超时 → OutcomeFailed+全点 bad；异常码 → Partial+对应点 bad
- [ ] LinkStatus{state,failures,last_success,last_error} 在 up/degraded/down 间按规则转换
- [ ] 单从站超时不阻塞同链路其他从站（并发断言）
- [ ] 写后回读的容差判定可供 09 调用
