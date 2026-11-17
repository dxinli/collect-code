# 05: MQTT 通道与属性上行

**What to build:** 采集到的点位能被发成一条符合契约的 MQTT 属性报文，并能从下行 Topic 收到一条服务调用。

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] MQTT 通道支持连接/重连/KeepAlive/LWT，断线时 Publish 返回可识别错误
- [ ] 属性上报按 config.adapter.topics 模板生成 Topic，JSON 字段与 Spec §4 一致
- [ ] snapshot 与 delta 两种模式；delta 去抖按 delta_debounce_ms 合并
- [ ] quality/ts 透传正确；属性超过 stale_after_s 时为 stale
- [ ] 订阅 service/invoke Topic 并能解析为服务调用（下行入口打通）
