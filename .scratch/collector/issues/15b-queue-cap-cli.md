# 15b: pending 上限与运维命令

**What to build:** 队列不会无限增长，现场能安全地手工清队列。

**Blocked by:** 15a,11

**Status:** ready-for-agent

- [ ] `queue.pending_max` 配置与校验；多 service profile 下超限淘汰最旧并记 cmd_dropped(queue_full)
- [ ] 连续 10 条同 service：设备只执行最新，cmd_dropped 预期条数明确
- [ ] `queue flush --device` 无 `--yes` 退出非 0 且不调 RPC；有 `--yes` 调一次并记日志
- [ ] 上限边界（=max、max+1）与跨设备隔离各有用例
