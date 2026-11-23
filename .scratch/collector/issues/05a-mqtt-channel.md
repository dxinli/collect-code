# 05a: MQTT 通道：连接、重连、LWT

**What to build:** 北向连接断开能自动恢复，断线时调用方拿到可识别错误。

**Blocked by:** 02a

**Status:** in-review

- [ ] 连接/重连/KeepAlive；节点级 LWT 的 topic/retain/payload 精确断言
- [ ] 断线时 Publish 返回 typed error（`errors.Is` 可判），不阻塞、不 panic
- [ ] 重连后自动重订阅下行 filter
- [ ] Topic 模板支持非默认 product_key/前缀（模板来自 config，不硬编码）

## Comments

- 2026-11-23 计划 v1–v4：v1 提交、独立评审（6 个阻断项）、v2 修订、用户“全部按推荐，计划评审通过”（Q1=B、Q9=A′、Q5=A 等）。计划见 `.scratch/collector/plans/05a.md`。
- 2026-11-23 实现完成，分支 `feat/05a-mqtt-channel`：`internal/topics` 模板渲染/校验、`internal/channels/mqtt`（paho + 显式重连/LWT/重订阅/typed error/有界背压）、`internal/testutil/mqttclient` 共用测试客户端。L1 假时钟；L2 用 06b 内嵌 broker 与 `/_sim/captured|invoke|control`。
- 验证：`go test -race ./...`、L1 `-count=10`、Close/Race `-count=20`、L2 `-count=3`、L2CloseNoLWT `-count=20`、`make ci` 全绿。
- 等待用户验收后再勾选验收项；`done` 只由合并后的 `tickets-sync` 写入。
