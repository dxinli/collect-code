# 02a: 内核接口与假时钟

**What to build:** 同一套内核代码能在假时钟与假通道下确定性运行，证明时间与传输真的可替换。

**Blocked by:** 01a

**Status:** ready-for-agent

- [ ] `interfaces` 冻结 Driver/Channel/UplinkAdapter/DownlinkAdapter/Clock/Store 与通用类型
- [ ] FakeClock 支持 Now/Advance/NewTicker/NewTimer/Sleep；覆盖到期顺序、Stop、零/负 duration、goroutine 泄漏
- [ ] 有一个最小内核组件实际通过注入的 Clock 取时间（证明接线，不是孤立实现）
- [ ] fake Channel/UplinkAdapter/DownlinkAdapter 可注入
- [ ] `internal/core/*` 不 import drivers/channels/adapters/bbolt；由 CI 静态检查强制执行
