# 03: 链路调度与设备状态机（假驱动）

**What to build:** 一台设备的属性在假时钟驱动下按相位稳定采集、判定新鲜度、判离线与恢复，全部行为不依赖真实等待与真实硬件。

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] 相位锁定：假时钟推进 24h，tick 计划时刻不累积漂移
- [ ] missed_tick 被记录且不补跑；单设备 read_deadline 到期即标 bad 并继续下一设备
- [ ] 新鲜度：超过 freshness_window 的样本为 stale；驱动给的 bad 不被升级为 good
- [ ] 连续 offline_after_intervals 个周期无新鲜观测 → device_offline 事件；恢复发 device_online + 全量快照
- [ ] 新鲜观测入口同时支持 Read 与 Event（推送型），窗口全部来自配置
