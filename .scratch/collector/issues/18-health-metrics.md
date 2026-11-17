# 18: 健康端点、指标与日志完善

**What to build:** 运维不登设备就能判断「活着没、连上没、积压多少」。

**Blocked by:** 03a,05a,15b,16a

**Status:** ready-for-agent

- [ ] `/healthz` 恒 200 仅表存活；`/status` 返回 store/MQTT/链路/在线/队列深度
- [ ] `/metrics` 指标名与标签固定，取值与内部计数一致
- [ ] 开启时仅绑 `health.addr`；不 bind 8080/1700（扫描监听端口断言）
- [ ] 日志分级/滚动/脱敏回归用例
