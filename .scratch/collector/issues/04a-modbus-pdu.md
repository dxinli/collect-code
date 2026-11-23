# 04a: Modbus PDU 与三种传输

**What to build:** 同一套 PDU 能在 TCP、串口 RTU、RTU-over-TCP 上收发，坏帧被正确拒绝。

**Blocked by:** 02a

**Status:** in-review

- [ ] 三种传输 framing 可切换，PDU 编解码共用
- [ ] golden PDU：03/04/06/16 与异常响应；CRC16 正确
- [ ] 坏 CRC、短帧、非法功能码被拒绝并归为协议错误
- [ ] 测试内置最小从站对端（in-test），不依赖 06a
