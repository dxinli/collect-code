# 追踪矩阵

本文件把每个 `REQ-*` 与合同锚点映射到测试或 ticket。状态只有三种：

- `covered`：已有测试或可执行检查作为证据。
- `planned(ticket)`：已有 ticket 承接，尚未实现。
- `not-applicable`：该条目不产生可执行测试，必须写明原因。

CI 只失败于“既无归属也无计划”的条目。实现 PR 落地测试后，必须把对应行改为 `covered` 并写明测试文件或检查脚本。

## 规范条目

| ID | 状态 | 证据 / 承接 |
|---|---|---|
| REQ-SCOPE-01 | covered | `docs/产品定型.md` §1–§3、§9 |
| REQ-TIME-01 | covered | 02a；`scripts/check-time.sh` |
| REQ-SEAM-01 | covered | 02a；`scripts/check-deps.sh` |
| REQ-SEAM-02 | planned(02b) | `internal/interfaces/store.go` 已冻结；bbolt 合同测试待 02b |
| REQ-QUALITY-01 | planned(03b) | `03b-state-freshness` |
| REQ-QUALITY-02 | planned(03b) | `03b-state-freshness` |
| REQ-QUALITY-03 | planned(03b) | `03b-state-freshness` |
| REQ-QUALITY-04 | planned(03b) | `03b-state-freshness` |
| REQ-QUALITY-05 | planned(03b) | `03b-state-freshness`；边界判据见 `docs/testing.md` |
| REQ-SCHED-01 | planned(03a) | `03a-scheduler-phase` |
| REQ-SCHED-02 | planned(03a) | `03a-scheduler-phase` |
| REQ-SCHED-03 | planned(03a) | `03a-scheduler-phase` |
| REQ-SCHED-04 | planned(03a) | `03a-scheduler-phase` |
| REQ-INVOKE-01 | planned(09) | `09-service-invocation` |
| REQ-INVOKE-02 | planned(10) | `10-idempotency-terminal` |
| REQ-INVOKE-03 | planned(14a) | `14a-lorawan-downlink-ab` |
| REQ-INVOKE-04 | planned(14a) | `14a-lorawan-downlink-ab` |
| REQ-INVOKE-05 | planned(14b) | `14b-lorawan-ack-timeout` |
| REQ-QUEUE-01 | planned(15a) | `15a-queue-merge` |
| REQ-QUEUE-02 | planned(15b) | `15b-queue-cap-cli` |
| REQ-QUEUE-03 | planned(15a) | `15a-queue-merge` |
| REQ-CACHE-01 | planned(16b) | `16b-replay-order` |
| REQ-CACHE-02 | planned(16a) | `16a-bounded-cache` |
| REQ-EVENT-01 | planned(11) | `11-eventbus` |
| REQ-EVENT-02 | planned(11) | `11-eventbus` |
| REQ-EVENT-03 | planned(11) | `11-eventbus` |
| REQ-OPS-01 | planned(17a) | `17a-reload-atomic`、`17b-reload-lifecycle` |
| REQ-OPS-02 | planned(18) | `18-health-metrics` |
| REQ-OPS-03 | covered | 01a；`internal/logging`、`internal/app/logging_agreement_test.go` |
| REQ-CLI-01 | planned(15b) | `run`/`config-check`/`version`/`simulate` 已实现；`snapshot` 与 `queue flush` 待后续 ticket |
| REQ-SIM-01 | planned(12b) | 06b 已覆盖 MQTT broker 与云端桩；Modbus 从站与 ChirpStack 桩待 06a/12a/12b |

## 合同锚点

| ID | 状态 | 证据 / 承接 |
|---|---|---|
| EXT-TOPIC-01 | planned(05a) | `05a-mqtt-channel` |
| EXT-PROP-01 | planned(05b) | `05b-uplink-adapter` |
| EXT-INVOKE-01 | planned(09) | `09-service-invocation` |
| EXT-SEQ-01 | planned(09) | `09-service-invocation` |
| EXT-CODE-01 | planned(09) | `09-service-invocation` |
| EXT-STAGE-01 | planned(14a) | `14a-lorawan-downlink-ab` |
| EXT-EVENT-01 | planned(11) | `11-eventbus` |
| EXT-DEDUP-01 | planned(11) | `11-eventbus` |
| EXT-TARGET-01 | planned(09) | `09-service-invocation` |
| EXT-LWT-01 | planned(05a) | `05a-mqtt-channel` |
| CFG-LAYERS-01 | covered | 01a；`internal/config/load_test.go` |
| CFG-VALIDATION-01 | covered | 01b/01c；`internal/config/validate_test.go`、golden corpus |
| CFG-V73 | covered | 01c；V70–V73 单测与 golden |
| CFG-RELOAD-01 | planned(17a) | `17a-reload-atomic`、`17b-reload-lifecycle` |
| STORE-ACCEPT-01 | planned(02b) | `02b-store-bbolt` |
| STORE-FINISH-01 | planned(02b) | `02b-store-bbolt` |
| STORE-LOOKUP-01 | planned(02b) | `02b-store-bbolt` |
| STORE-LASTSEQ-01 | planned(02b) | `02b-store-bbolt` |
| STORE-DEVICES-01 | planned(02b) | `02b-store-bbolt` |
| STORE-SNAPSHOT-01 | planned(02b) | `02b-store-bbolt` |
| STORE-EXPIRY-01 | planned(02b) | `02b-store-bbolt` |
| STORE-CLOSE-01 | planned(02b) | `02b-store-bbolt` |
| SIM-CLI-01 | covered | 06b；`internal/sim/run_test.go`、`internal/app/simulate_test.go` |
| SIM-MANIFEST-01 | covered | 06b；`internal/sim/manifest_test.go` |
| SIM-CAPTURE-01 | covered | 06b；`internal/sim/capture_test.go` |
| SIM-INVOKE-01 | covered | 06b；`internal/sim/mqtt_test.go` |
| SIM-CONTROL-01 | covered | 06b；`internal/sim/control_test.go`、`internal/sim/mqtt_test.go` |
| SIM-SECURITY-01 | covered | 06b；`internal/sim/mqttclient_test.go`（独立 MQTT 客户端） |
