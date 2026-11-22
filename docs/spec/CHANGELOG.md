# 规范包变更记录

本文件记录规范包（`docs/spec/spec.md` + `docs/contracts/**`）的版本变更。行为变更必须在此列出涉及的 `REQ-*` 与合同锚点；编辑性修订也必须升 patch 并记录。

## 3.0.0 — 2026-11-22

> 3.0.0 尚未合并或发布；评审修订折叠在本条目，合并后不再追加。

**类型**：major（规范结构重写；行为不变）。

- 以 `docs/spec/spec.md` 取代 `.scratch/collector/spec.md` 的 v2-draft；旧内容冻结于 tag `spec-v2-draft`。
- 建立稳定 `REQ-*`：`REQ-SCOPE-01`、`REQ-TIME-01`、`REQ-SEAM-01..02`、`REQ-QUALITY-01..05`、`REQ-SCHED-01..04`、`REQ-UPLINK-01`、`REQ-INVOKE-01..05`、`REQ-QUEUE-01..03`、`REQ-CACHE-01..02`、`REQ-EVENT-01..03`、`REQ-OPS-01..03`、`REQ-CLI-01`、`REQ-SIM-01`。
- 评审修订：补回 LoRaWAN 上行去重语义 `REQ-UPLINK-01`、LoRa 按上报窗口计时的语义，以及 `/metrics` 的固定指标名与标签。
- 评审决策：5044 扩写为“未获得有效响应”；`cmd_dropped` 仅在实际撤销/超限时记录；固定指标标签取值；`LastSeq` 水位明确为冻结语义；上行去重窗口保持固定软件常量。
- 拆分合同：`docs/contracts/external.md`、`config.md`、`store.md`、`simulator.md`。
- 拆分支撑文档：`docs/testing.md`、`docs/traceability.md`、`docs/architecture.md`。
- 删除旧规范中的实现代码、配置样例、报文样例、决策编号、评审记录与 ticket 编号；这些事实分别归入代码、合同、ADR 与 tickets。
- 不改变产品行为、接口签名、配置字段、错误码或事件语义。
