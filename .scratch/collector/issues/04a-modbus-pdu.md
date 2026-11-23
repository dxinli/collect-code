# 04a: Modbus PDU 与三种传输

**What to build:** 同一套 PDU 能在 TCP、串口 RTU、RTU-over-TCP 上收发，坏帧被正确拒绝。

**Blocked by:** 02a

**Status:** done

- [ ] 三种传输 framing 可切换，PDU 编解码共用
- [ ] golden PDU：03/04/06/16 与异常响应；CRC16 正确
- [ ] 坏 CRC、短帧、非法功能码被拒绝并归为协议错误
- [ ] 测试内置最小从站对端（in-test），不依赖 06a

## Comments

- 2026-11-23 计划 v1 → v2（独立评审修订）→ v3（用户批准）→ v4（实施完成）；PR #17 已创建，ticket 置 `in-review`。实现提交、验证结果见 `.scratch/collector/plans/04a.md` §11。
- 串口 opener 未在本 ticket 实现；按已批准计划 4.16-A 留给后续 `04c-serial-opener`。04b 的 `modbus-rtu` 生产路径在其合并前阻塞。
