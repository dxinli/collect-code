# 06b: 内嵌 MQTT broker 与云端桩

**What to build:** 一条命令起可被真实二进制连接的假云端，并能注入下行。

**Blocked by:** 01a

**Status:** ready-for-agent

- [ ] 内嵌纯 Go broker 监听 `127.0.0.1:0`；`--manifest` 输出实际地址
- [ ] 云端桩订阅上行并可通过 `GET /_sim/captured` 查询
- [ ] `POST /_sim/invoke` 注入完整下行报文
- [ ] `broker_disconnect` 可模拟断线并恢复
- [ ] 无外部依赖（不要求 docker/apt）
