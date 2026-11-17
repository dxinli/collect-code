# 05a: MQTT 通道：连接、重连、LWT

**What to build:** 北向连接断开能自动恢复，断线时调用方拿到可识别错误。

**Blocked by:** 02a

**Status:** ready-for-agent

- [ ] 连接/重连/KeepAlive；节点级 LWT 的 topic/retain/payload 精确断言
- [ ] 断线时 Publish 返回 typed error（`errors.Is` 可判），不阻塞、不 panic
- [ ] 重连后自动重订阅下行 filter
- [ ] Topic 模板支持非默认 product_key/前缀（模板来自 config，不硬编码）
