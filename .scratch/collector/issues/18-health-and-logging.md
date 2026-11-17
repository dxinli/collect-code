# 18: 健康端点、指标与日志

**What to build:** 运维能在不登设备的情况下探活、看链路与队列状态、按字段 grep 日志，且凭据不被打印。

**Blocked by:** 03

**Status:** ready-for-agent

- [ ] health.enabled 时绑定 127.0.0.1:9100，提供 /healthz、/status、/metrics
- [ ] status 含链路状态、设备在线状态、队列深度；metrics 计数与内部一致
- [ ] 日志 JSON 结构化，字段含 device/point/seq/link/reason；敏感字段脱敏
- [ ] 启动校验不占用 8080/1700
