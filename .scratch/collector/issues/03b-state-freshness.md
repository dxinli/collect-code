# 03b: 设备状态、新鲜度与离线

**What to build:** 旧值不会被当成新值，离线/恢复判定可逐行验证。

**Blocked by:** 02a,03a

**Status:** ready-for-agent

- [ ] 按 Spec §5.1 真值表逐行测试（OK/Partial/全超窗/Failed/Unsupported/非法 payload/txack/uplink）
- [ ] `effective_stale_after = point.stale_after_s ?? freshness_window_s`；边界用 window±1ns
- [ ] 连续 offline_after_intervals 个周期无推进 → offline 事件；恢复 → online + 全量快照
- [ ] Read 与 Event 两个新鲜观测入口各测一遍；内核不改写驱动给的 bad 为 good
