# 06: 模拟器 v0.1：RTU 从站 + 内嵌 broker + 云端桩

**What to build:** 一条命令起一套可被真实二进制连接的假现场：假从站、假 ChirpStack、假云端，全部在进程内，无外部依赖。

**Blocked by:** 04

**Status:** ready-for-agent

- [ ] `collect-code simulate` 单进程启动 Modbus TCP 从站、RTU-over-TCP 从站、内嵌 MQTT broker、云端桩
- [ ] 从站寄存器布局来自 profile/台账中的 ASSUMED 映射，可配置站号与初始值
- [ ] 云端桩可打印收到的上行报文，并可按命令发送下行 invoke
- [ ] 端口使用 127.0.0.1:0 或可配置；启动后打印各端点地址，便于引擎连接
- [ ] 无外部依赖（不要求 docker/apt）
