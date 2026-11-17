# 06a: Modbus 从站与故障控制

**What to build:** 一个独立实现寄存器语义的假从站，可被单独停摆/变慢/返回坏帧。

**Blocked by:** 04a

**Status:** ready-for-agent

- [ ] TCP 与 RTU-over-TCP 从站，多 unit_id 并存
- [ ] 独立实现站号路由、功能码、地址、数量、CRC、异常码
- [ ] 测试用独立 Modbus 客户端断言请求日志（不复用被测驱动）
- [ ] `POST /_sim/control` 支持 per-unit offline/slow/bad_crc
- [ ] `GET /_sim/requests` 返回完整请求日志
