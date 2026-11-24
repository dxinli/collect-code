# 规范包变更记录

本文件记录规范包（`docs/spec/spec.md` + `docs/contracts/**`）的版本变更。行为变更必须在此列出涉及的 `REQ-*` 与合同锚点；编辑性修订也必须升 patch 并记录。

## 3.2.0 — 2026-11-24

**类型**：minor（新增模拟器 Modbus 从站合同；不改变产品行为与既有接口语义）。

- 新增 `SIM-MODBUS-01`：Modbus TCP / RTU-over-TCP 从站、站号路由、功能码 03/04/06/16、异常码、寄存器语义与独立实现要求。
- 新增 `SIM-REQUESTS-01`：`GET /_sim/requests` 请求日志的 JSON 形状、`seq`/`dropped` 与有界容量。
- 扩展 `SIM-CLI-01`：新增 `--modbus-addr`、`--modbus-rtu-addr` 与可选 `--config`；说明 `--config` 只影响 Modbus 模块。
- 扩展 `SIM-MANIFEST-01`：新增 `modbus` section、`endpoints.requests`、`control_actions.modbus` 与 `modules` 元素。
- 扩展 `SIM-CONTROL-01`：新增 `unit_offline` / `unit_slow` / `unit_bad_crc` 的请求、响应与惰性恢复语义。
- 扩展 `SIM-SECURITY-01`：新监听器的 loopback 约束与独立 Modbus 客户端要求。
- `REQ-SIM-01` 的 Modbus 部分由 06a 承接；`REQ-*` 语义不变。

## 3.1.2 — 2026-11-24

**类型**：patch（修正 3.1.1 遗漏的 Version 行同步）。

- `docs/spec/spec.md` 的 `Version` 行同步到当前包版本 `3.1.2`；3.1.1 的其余编辑性修订不变。

## 3.1.1 — 2026-11-24

**类型**：patch（编辑性修订；不改变行为、接口、配置或错误码）。

- 移除 `docs/spec/spec.md` 标题中的重复版本号；版本唯一来源为 `docs/spec/VERSION`。
- 修正 3.0.0 条目中“当前规范包版本”的历史表述。

## 3.1.0 — 2026-11-23

**类型**：minor（增补 Store 合同语义；不改接口签名与产品行为）。

- 解析 02b 的三个未决项：`Close` 后行为、`SaveSnapshot(nil)`、`Quality`/`Phase` 读取校验，均采纳 A。
- 明确 `STORE-CLOSE-01`：`Close` 幂等；Close 后带 ctx 方法返回 `ErrClosed`。
- 明确 `STORE-SNAPSHOT-01`：`SaveSnapshot(nil)` 等价空快照；`Quality` 原样透传；保留属性顺序。
- 明确持久化值语义：JSON 往返与精度边界。
- 明确输入前置条件：`device` 非空；`seq` 不校验业务单调性。
- 明确损坏记录：读取 fail-fast；`DeleteExpired` 回滚。
- 移除 `docs/contracts/store.md` 的 02b pending 表；02b 解冻后可继续实现。

## 3.0.0 — 2026-11-22

> 3.0.0 是当时的规范包版本；评审修订折叠在本条目，发布后不再追加。

**类型**：major（规范结构重写；行为不变）。

- 以 `docs/spec/spec.md` 取代 `.scratch/collector/spec.md` 的 v2-draft；旧内容冻结于 tag `spec-v2-draft`。
- 建立稳定 `REQ-*`：`REQ-SCOPE-01`、`REQ-TIME-01`、`REQ-SEAM-01..02`、`REQ-QUALITY-01..05`、`REQ-SCHED-01..04`、`REQ-UPLINK-01`、`REQ-INVOKE-01..05`、`REQ-QUEUE-01..03`、`REQ-CACHE-01..02`、`REQ-EVENT-01..03`、`REQ-OPS-01..03`、`REQ-CLI-01`、`REQ-SIM-01`。
- 评审修订：补回 LoRaWAN 上行去重语义 `REQ-UPLINK-01`、LoRa 按上报窗口计时的语义，以及 `/metrics` 的固定指标名与标签。
- 评审决策：5044 扩写为“未获得有效响应”；`cmd_dropped` 仅在实际撤销/超限时记录；固定指标标签取值；`LastSeq` 水位明确为冻结语义；上行去重窗口保持固定软件常量。
- 评审补遗：`SaveDevices` 不得修改幂等水位；4001 设备侧拒绝时说明 `data.field`；删除队列规则重复表述。
- 拆分合同：`docs/contracts/external.md`、`config.md`、`store.md`、`simulator.md`。
- 拆分支撑文档：`docs/testing.md`、`docs/traceability.md`、`docs/architecture.md`。
- 删除旧规范中的实现代码、配置样例、报文样例、决策编号、评审记录与 ticket 编号；这些事实分别归入代码、合同、ADR 与 tickets。
- 不改变产品行为、接口签名、配置字段、错误码或事件语义。
