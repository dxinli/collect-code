# 04: Modbus 驱动（三种传输）

**What to build:** 一台 Modbus 设备（TCP / 串口 / 串口服务器任一种）能被按 profile 读取与写入，坏帧与超时被正确归到质量或链路状态，且不拖累同链路其他设备。

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] 同一 PDU 编解码支持 TCP、串口 RTU、RTU-over-TCP 三种传输，配置可切换
- [ ] 按 profile 点位批量读（03/04），类型/缩放/字节序正确；坏帧（CRC 错、短帧、异常码）映射为 quality=bad 或 OutcomeFailed
- [ ] 写点位使用 profile 声明的功能码与地址；写后回读按 verify 容差判定
- [ ] 单从站超时不影响同链路其他从站；链路状态 up/degraded/down 可观测
- [ ] 与 RTU-over-TCP 从站对端联调通过
