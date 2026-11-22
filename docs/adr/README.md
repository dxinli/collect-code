# ADR 索引与决策索引

本文件只做导航，不是规范，也不是“旧编号 → 新锚点”的解析表。需要当前行为时直接读 `docs/spec/spec.md` 与 `docs/contracts/`。

## ADR 索引

| ADR | 决策 | 状态 |
|---|---|---|
| [0001](0001-three-extension-points-as-interfaces.md) | 三个扩展点用接口定义，依赖只向内核收敛 | Accepted |
| [0002](0002-chirpstack-integration-boundary.md) | ChirpStack v4：上行走 MQTT integration，下行走 gRPC | Accepted |
| [0003](0003-freshness-and-link-phase-scheduling.md) | 新鲜度由内核计算，调度以链路相位锁定 | Accepted + Amendment 2026-11-22 |
| [0004](0004-idempotent-service-invocation.md) | 至多一次受理 + 幂等去重 + 分级确认，状态走 bbolt | Accepted + Amendment 2026-11-22 |
| [0005](0005-downlink-queue-governance.md) | 下行队列用 `GetQueue` + 精确撤销，不用 `FlushQueue` 合并 | Accepted + Amendment 2026-11-22 |
| [0006](0006-doc-authority-and-spec-change-policy.md) | 文档按事实类型分权，规范包版本化并独立变更 | Accepted |

## D 决策索引

状态含义：`current` 仍是当前决策；`fact` 是已核实的外部事实；`scope` 是产品范围边界；`duplicate` 与另一条决策同义，保留仅为历史可读。

| D | 一行决策 | 落点 | 状态 |
|---|---|---|---|
| D1 | 三个扩展点全部为接口，内核不 import 实现 | ADR-0001 | current |
| D2 | 装配只发生在 `cmd` | ADR-0001 | current |
| D3 | 服务调用至多一次受理 + 幂等 `(device,seq)` | ADR-0004 / spec | current |
| D4 | 幂等状态在 bbolt，两阶段 `accepted → final` | ADR-0004 / store contract | current |
| D5 | LoRa 分级确认 A/B/C，仅 C 级算成功 | spec / external contract | current |
| D6 | LoRa 超时按上报窗口数计算 | spec | current |
| D7 | 调度单位是链路，设备是链路内任务 | ADR-0003 / spec | current |
| D8 | 缓存分两级：事件有界队列 + 属性最新快照 | spec | current |
| D9 | 一台设备只声明一条接入路径，双路径属 v2 | 产品定型 §3 | scope |
| D10 | 点位映射只在配置，不在代码 | config contract | current |
| D11 | 默认不监听 TCP；健康端点默认关闭且只绑 loopback | spec | current |
| D12 | 时间只经注入的 `Clock` 进内核 | ADR-0003 / spec | current |
| D13 | 日志为 JSON 结构化，字段固定 | spec | current |
| D14 | 队列合并用 `GetQueue` 精确撤销，不用 `FlushQueue` | ADR-0005 / spec | current |
| D15 | `Events()` 是 `Driver` 一等方法 | ADR-0001 | current |
| D16 | 新鲜度由内核计算，`last_success_at` 只由新鲜观测推进 | ADR-0003 / spec | current |
| D17 | 命令状态只前进 `accepted → delivered → completed/failed` | ADR-0004 / spec | current |
| D18 | 适配器拆上行/下行两个接口 | ADR-0001 | current |
| D19 | 调度器相位锁定，missed tick 不补跑，单设备 deadline | ADR-0003 / spec | current |
| D20 | 幂等与状态用 bbolt | ADR-0004（与 D4 同义） | duplicate |
| D21 | `seq` 为 uint64、按设备单调递增、由云端分配 | external contract | current |
| D22 | 同设备同 service 的 pending 下行始终合并、只留最新 | ADR-0005 / spec | current |
| D23 | Modbus 写一次 + 写后回读，不重发 | ADR-0004 / spec | current |
| D24 | 配置分实例配置与模型配置两档 | 产品定型 §1 / config contract | current |
| D25 | ChirpStack 上行 MQTT、下行 gRPC、队列用 GetQueue | ADR-0002 | fact |
| D26 | 链路/驱动故障一律 `5040`，不产生 `cmd_timeout` | ADR-0004 / external contract | current |
| D27 | 两阶段幂等；重试必须换新 `seq`，窗口小于 TTL | ADR-0004 / external contract | current |
| D28 | LoRa payload 只支持声明式位域 | config contract | scope |

## 维护规则

- 新决策写新 ADR，并在本索引加一行。
- 推翻旧决策时新增 ADR，把旧行状态改为 `superseded by ADR-NNNN`；不删除旧行。
- 行为细节不写进本文件；`落点` 只指向当前权威。
