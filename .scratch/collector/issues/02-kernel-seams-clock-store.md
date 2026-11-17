# 02: 内核缝隙：接口、Clock、Store

**What to build:** 内核拥有可替换的时间与持久化：同一套内核代码在假时钟 + 内存存储下可确定性运行，在生产时用真实时钟 + bbolt，切换不改内核。

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] `internal/interfaces` 定义 Driver/Channel/UplinkAdapter/DownlinkAdapter/Clock/Store 与通用类型
- [ ] `Clock` 含 Now/NewTicker/NewTimer/Sleep；测试用 FakeClock 可 Advance 并触发到期 ticker/timer
- [ ] bbolt Store 实现 BeginAccepted/Finish/Lookup/LastSeq/LoadDevices/SaveDevices，内存实现供内核测试
- [ ] 两阶段提交测试：BeginAccepted → Close → 重开 → Lookup 返回 accepted
- [ ] `go vet`/lint 规则可检测内核与驱动中的直接 time.Now/Sleep/NewTicker/After（至少 code review 清单固化）
