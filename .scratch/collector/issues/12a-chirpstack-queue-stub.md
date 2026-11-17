# 12a: ChirpStack 桩：官方 proto 与队列

**What to build:** 桩用官方 proto 实现，引擎对它的调用与真实 ChirpStack 兼容。

**Blocked by:** 01a

**Status:** ready-for-agent

- [ ] 使用 `github.com/chirpstack/chirpstack/api/go/v4@v4.20.0` 实现 DeviceServiceServer
- [ ] Enqueue/GetQueue/FlushQueue 语义；队列内存持久，isPending 可查
- [ ] 断言请求字段 devEui/confirmed/fPort/data/object.ref；校验 Bearer token
- [ ] 暴露 RPC 调用日志（供 15 断言 FlushQueue 调用数）
