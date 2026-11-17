# Spec：智能采集应用（Collector）

- 状态：ready-for-agent（待评审）
- 日期：2026-11-17
- 上游：`docs/产品定型.md`、`docs/adr/0001`–`0005`、`GLOSSARY.md`、`docs/缺口台账.md`
- 词汇以 `GLOSSARY.md` 为准；决策以 `docs/adr/` 为准，本文件不重复推导，只做可拆 ticket 的收敛。

---

## Problem Statement

设备厂商有两类边缘现场，都要「把设备数据采上来、把云端指令发下去」：

1. **LoRaWAN 网关（MQ-SG40W）**：现场有 ChirpStack v4，需要旁路消费它的上行事件、并经它下发 LoRa 下行；同时网关下还有 Modbus TCP 电动调节阀，要按寄存器表采集与写入。网关原有上云链路**必须保留**。
2. **RS485 融合终端**：一条 485 总线挂多台 Modbus RTU 传感器，读上来后用 MQTT 上报平台。

两类现场的**共同内核**是「南向按协议采/控、北向 MQTT JSON、物模型统一」。现状是没有统一的采集应用：协议各写各的、字段各定各的、下行可靠性（超时/重发/离线）没有统一语义，导致每个现场都从零搭一遍，且 LoRa 下行的「送达 ≠ 执行」极易被误判。

要交付一个**一套二进制、按配置装配、接口化可扩展**的采集引擎，把两类现场收敛到同一套代码与同一套配置模型。

## Solution

一个部署在边缘的**采集引擎**，南向北向正交：

- **南向**：三个 `Driver` 实现——`modbus-tcp`、`modbus-rtu`、`lorawan`（上行走 ChirpStack MQTT integration，下行走 gRPC `DeviceService/Enqueue`）。
- **内核**：链路相位调度、设备状态机（在线/离线/质量）、服务调用生命周期（至多一次受理 + 幂等 + 分级确认 + 队列治理）、北向断线缓存（有界）。
- **北向**：一个 `Channel` 实现 `mqtt`。
- **物模型**：`UplinkAdapter` / `DownlinkAdapter` 实现 `irrigation-v1`，Topic 与点位映射全部配置化。
- **持久化**：嵌入式 KV `bbolt` 存幂等表、在线状态、属性快照。
- **可联调**：内置模拟器子命令，无实物也能验收全链路。

部署形态：`linux/arm64`（另保 `amd64` 联调），单二进制 + YAML + systemd，`CGO_ENABLED=0` 静态编译。

## User Stories

> actor 分四类：现场部署工程师（D）、云端集成工程师（C）、运维（O）、研发联调（R）。

1. As D, I want 一个二进制 + 一份 YAML 即可启动，so that 我在网关或融合终端上都能用同一套部署步骤装起来。
2. As D, I want 一个 `config-check` 子命令，so that 我改完配置后能立刻知道哪里错，而不是等服务起不来才排查。
3. As D, I want 改「设备站号 / 采集周期 / 上报周期 / Broker 地址 / Topic 前缀」只改 YAML 并 SIGHUP 生效，so that 现场调参不用重新编译或重启整个服务。
4. As D, I want 换设备型号只新增/替换一份 profile YAML，so that 换阀门或传感器型号不碰代码。
5. As C, I want 设备属性以统一 JSON 上报（快照 + 增量），so that 我的云端画曲线、对账都有基线。
6. As C, I want 每个属性都带 `quality`（good/stale/bad）与真实采样时间 `ts`，so that 我不会把「没读到的旧值」当成「刚读到的正常值」。
7. As C, I want 下行服务调用有 `service_id` + `seq` 且同 `seq` 幂等，so that 我重发同一条指令不会让阀门动作两次。
8. As C, I want 服务回执区分「网关已发射 / 设备已收到 / 业务已完成」，so that LoRa 下行我不会把「发出了」误当「阀门动了」。
9. As C, I want 服务调用在设备离线时也给出明确结果（而不是无限等待），so that 我云端能决定是重试还是告警。
10. As C, I want 同一阀门连续下发多次开度指令时设备最终只执行最新值，so that 阀门不会做无意义的中间动作。
11. As C, I want 设备离线/恢复、阀门故障、指令超时都以事件上报，so that 我能驱动告警而不是轮询推断。
12. As O, I want 结构化日志字段固定（device/point/seq/link），so that 我用 grep 就能定位问题，不靠读大段文本。
13. As O, I want 一个只读健康端点（默认关，可开），so that 现场探活不用登设备看日志。
14. As O, I want 断网时事件与最新快照有界缓存、恢复后补发，so that 短暂断网不丢关键事件、也不会写满磁盘。
15. As R, I want `simulate` 一个命令同时起 Modbus 从站 + ChirpStack 桩 + MQTT 云端桩，so that 没有实物我也能跑通上行、下行、超时、断线四条链路。
16. As R, I want 内核单测用假驱动 + 假时钟跑，so that 调度与状态机逻辑不需要真实硬件就能验证。

## Implementation Decisions

（决策细节见 ADR，这里只列构建所需的结构性结论）

### 模块与包（见 `docs/产品定型.md` §7）

| 包 | 职责 |
|---|---|
| `internal/interfaces` | `Driver` / `Channel` / `UplinkAdapter` / `DownlinkAdapter`、命令状态、通用类型 |
| `internal/config` | YAML 解析、校验、差别重载 |
| `internal/core/scheduler` | 链路相位调度（D19/ADR-0003） |
| `internal/core/state` | 设备状态机、属性快照、新鲜度统一计算 |
| `internal/core/invoker` | 服务调用生命周期、幂等、分级确认、队列治理（ADR-0004/0005） |
| `internal/core/forward` | 北向有界缓存与补发（D8） |
| `internal/core/eventbus` | 进程内事件去重与分发 |
| `internal/core/store` | bbolt 封装：幂等表 + 状态快照（ADR-0004 决策 3/4） |
| `internal/core/status` | 只读健康端点 |
| `internal/drivers/modbus` | TCP/RTU 共用 PDU 编解码，两套传输 |
| `internal/drivers/lorawan` | ChirpStack gRPC 客户端 + MQTT 事件订阅 |
| `internal/channels/mqtt` | paho 封装、重连、LWT |
| `internal/adapters/irrigation_v1` | 物模型编解码（上/下行） |
| `internal/simulator` | Modbus 从站、ChirpStack 桩、MQTT 桩 |
| `cmd/collect-code` | 装配 + 子命令（`run`/`simulate`/`config-check`/`version`/`queue flush`） |

### 关键接口（见 `docs/产品定型.md` §6，此处为构建契约）

- `Driver`：`Name/Open/Close/Capabilities/Read/Write/Events/Status`。`Capabilities{Pollable, SyncVerify, AsyncAck, Confirmed}`；`Events() <-chan DriverEvent` 为一等方法（拉取型返回 `nil`）；`Read` 返回 `([]Sample, Outcome)`，`Outcome` 含 `Partial/Failed/Unsupported`。
- `Sample{Quality 初值, At}`：最终 `good/stale` 由内核用 `Sample.At` 与 `freshness_window` 计算，内核不把 `bad` 升级为 `good`（ADR-0003）。
- 命令状态机 `CommandHandle{Ref, State}`，`State ∈ accepted→delivered→completed/failed`，只前进。
- `UplinkAdapter`（`PropertyReport/EventReport/DecodePayload`）与 `DownlinkAdapter`（`ParseInvocation/ServiceReply`）分开。

### 配置模型（YAML 分层）

```
config.yaml    # 节点级：node.id、北向 mqtt、日志、时限、缓存、bbolt 路径
links.yaml     # 南向链路：modbus-tcp / serial / chirpstack
profiles/*.yaml# 设备类型 → 点位映射（address/reg/func_code/data_type/scale/unit/rw）
devices.yaml   # 设备实例：SN、device_type、profile、所在链路、周期
```

点位映射是**配置不是代码**（D10）；payload 位域仅支持声明式描述（D28）；Topic 模板在适配器配置里（D24）。

### 报文契约（`docs/产品定型.md` §10）

- Topic：`v1/{product_key}/{device_sn}/property/post`、`/service/invoke`、`/service/reply`、`/event/{event_id}`、节点级 LWT `v1/{product_key}/$nodes/{node_id}/state`。
- 属性报文必带 `mode`、`online`、`quality`、采样 `ts`。
- 服务回执必带 `code`、`seq`、`data.stage`、`replay`。
- `seq` 契约：uint64、按设备单调、云端分配、重试换新 `seq`、重试窗口 < 幂等 TTL（7 天）。

### 可靠性语义（ADR-0004/0005）

- 至多一次受理 + `(device,seq)` 幂等；`4003` 回放已知终态，`4004` 拒绝未知终态。
- 分级确认 A/B/C；只有 C 级回 `code=0`；`5041` 带 `data.stage`；链路 down 回 `5040` 不产 `cmd_timeout`。
- 同 service 下行始终合并只留最新；队列治理用 `GetQueue` 精确撤销，**不用 `FlushQueue`**。

## Testing Decisions

### 缝隙（seam）

- **唯一内核边界** = `internal/interfaces` 四个接口。内核（scheduler/state/invoker/forward）的所有测试都注入假 `Driver`/`Channel`/`Adapter` + 假 `Clock`，不 import 任何具体驱动。
- **集成边界** = `internal/simulator` + 假 `Clock`，跑真实 `cmd/collect-code` 的端到端链路。

### 什么是好测试（只测外部行为，不测实现细节）

- 对内核：喂入「假驱动在某时刻返回某样本/事件」，断言「上报了哪些属性、质量如何、设备在线状态如何、发了什么事件」，**不**断言内部 goroutine 数量、channel 长度、锁。
- 对调度：注入假 Clock 推进时间，断言「相位锁定下 tick 时刻不累积漂移」「单设备 deadline 到期即放弃该设备」，**不**依赖真实 `time.Sleep`。
- 对幂等：断言「同 seq 重发不执行第二次」「kill -9 后重启该 seq 回 4004」。
- 对协议：只测「合法帧 ↔ 结构体」的编解码往返 + 已知坏帧（CRC 错、短帧、异常码）的拒绝，不 mock 网络库内部。

### 测什么模块

| 模块 | 测试重点 | 缝隙 |
|---|---|---|
| `core/scheduler` | 相位锁定、missed_tick 不补跑、单设备 deadline | 假 Driver + 假 Clock |
| `core/state` | 新鲜度计算、质量不降级、在线翻转事件 | 假 Driver |
| `core/invoker` | 幂等、4003/4004、分级确认、同 service 合并 | 假 Driver + bbolt 临时文件 |
| `core/forward` | 有界淘汰、补发顺序 | 假 Channel |
| `core/store` | bbolt 两阶段提交、崩溃恢复（写一半后重开） | 真实 bbolt（临时目录） |
| `drivers/modbus` | PDU 编解码、超时映射到 Outcome、写后回读 | 内存 TCP 对端 / 假串口 |
| `drivers/lorawan` | ChirpStack 事件解码（base64）、enqueue 参数 | ChirpStack 桩 |
| `channels/mqtt` | 重连、LWT、断线时 Publish 报错被 forward 捕获 | 假 MQTT broker |
| `adapters/irrigation_v1` | 属性/事件/服务往返、非法报文拒绝 | 纯函数 |
| 端到端 | 24 h 稳定上报、下行回执分类、断线补发 | `simulate` + 假 Clock |

### 先验（prior art）

无既有代码可参照（greenfield）。模拟器是唯一「真实对端」的先验来源，因此 **simulator 必须与 profile/点位映射共用同一份配置**，保证「模拟的就是被测的」。

## Out of Scope

（见 `docs/产品定型.md` §3 非目标 N1–N9）

- 云端后台页面、数据库；修改 ChirpStack 源码；硬件驱动；设备固件 OTA。
- 本地 Web UI / 配置界面。
- 多节点编排、云端配置下发。
- 定时灌溉策略、轮灌计划、规则引擎。
- 主备双接入路径自动切换与对账。
- 一机一密、远程凭据轮换。
- LoRa payload 的压缩/条件/变长编码（v1 只支持声明式位域）。

## Further Notes

- 版本路线（`docs/产品定型.md` §14）：v0.1 = 485→MQTT 全链路（需求 2）；v0.2 = Modbus TCP 阀门 + 服务调用 + 幂等；v0.3 = LoRaWAN 上下行 + 分级确认 + 队列治理；v0.4 = 可靠性/运维；v1.0 = 实物联调 + 交付。
- 所有硬件相关数值都是 `ASSUMED`，见 `docs/缺口台账.md`；真实资料到位只替换配置与夹具，不改内核。
- `seq` 契约、分级确认级别、`ASSUMED` 清单索引必须进入交付文档（验收 §12.9）。
- 实物联调的 P0 阻塞项：阀门写寄存器语义（幂等 vs 动作）、Modbus 寄存器表、ChirpStack 凭据与 integration、DevEUI/payload 规范。
