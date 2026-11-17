# Spec：智能采集应用（Collector）

- 状态：v2-draft（两轮 Spec 评审后修订，待用户评审）
- 日期：2026-11-17
- 上游：`docs/产品定型.md`、`docs/adr/0001`–`0005`、`GLOSSARY.md`、`docs/缺口台账.md`
- 词汇以 `GLOSSARY.md` 为准；决策以 `docs/adr/` 为准。

---

## Problem Statement

设备厂商有两类边缘现场，都要「把设备数据采上来、把云端指令发下去」：

1. **LoRaWAN 网关（MQ-SG40W）**：现场有 ChirpStack v4，需要旁路消费它的上行事件、并经它下发 LoRa 下行；同时网关下还有 Modbus TCP 电动调节阀，要按寄存器表采集与写入。网关原有上云链路**必须保留**。
2. **RS485 融合终端**：一条 485 总线挂多台 Modbus RTU 传感器，读上来后用 MQTT 上报平台。

两类现场的**共同内核**是「南向按协议采/控、北向 MQTT JSON、物模型统一」。现状是没有统一的采集应用：协议各写各的、字段各定各的、下行可靠性（超时/重发/离线）没有统一语义，导致每个现场都从零搭一遍，且 LoRa 下行的「送达 ≠ 执行」极易被误判。

要交付一个**一套二进制、按配置装配、接口化可扩展**的采集引擎，把两类现场收敛到同一套代码与同一套配置模型。

## Solution

一个部署在边缘的**采集引擎**，南向北向正交：

- **南向**：两个 `Driver` 实现、四种链路类型——`modbus`（传输可为 `modbus-tcp` / `modbus-rtu` 串口 / `modbus-rtu-tcp` 串口服务器）与 `lorawan`（上行走 ChirpStack MQTT integration，下行走 gRPC `DeviceService/Enqueue`）。
- **内核**：链路相位调度、设备状态机、服务调用生命周期（至多一次受理 + 幂等 + 分级确认 + 队列治理）、北向断线缓存。
- **北向**：一个 `Channel` 实现 `mqtt`。
- **物模型**：`UplinkAdapter` / `DownlinkAdapter` 实现 `irrigation-v1`，Topic、点位、服务全部配置化。
- **持久化**：嵌入式 KV `bbolt`（决策 4=B），存幂等表、在线状态、属性快照。
- **可联调**：内置模拟器子命令，无实物也能验收全链路。

部署形态：`linux/arm64`（另保 `amd64` 联调），单二进制 + YAML + systemd，`CGO_ENABLED=0` 静态编译。

## In Scope

v0.1–v1.0 全量：

- 两个南向驱动、四种链路类型（modbus-tcp / modbus-rtu / modbus-rtu-tcp / lorawan）、一个北向 MQTT 通道、一套物模型适配器。
- 配置化点位/服务/事件/Topic，`config-check` 校验，SIGHUP 差别重载。
- 设备状态机、质量与新鲜度、离线/恢复事件。
- 服务调用：幂等、分级确认、队列治理、写后回读、按窗口超时。
- 北向断线有界缓存与补发。
- 结构化日志、只读健康端点、bbolt 状态库。
- 内置模拟器（Modbus TCP/RTU 从站、ChirpStack 桩、MQTT broker+云端桩）。
- 交付文档包与构建脚本。

## User Stories

actor：D=现场部署工程师，C=云端集成工程师，O=运维，R=研发联调。

### 部署与配置

1. As D, I want 一个二进制 + 一份 YAML 目录即可启动，so that 网关与融合终端用同一套部署步骤。
2. As D, I want `collect-code config-check --config /etc/collect-code`，so that 改完配置立刻知道哪里错。
3. As D, I want 改设备站号/采集周期/上报周期/Broker/Topic 前缀只改 YAML 并 SIGHUP 生效，so that 现场调参不用重编译。
4. As D, I want 解析失败时整体不应用并给出行列号，旧配置继续运行，so that 坏配置不会把现场搞停。
5. As D, I want 移除设备后上报一次 `device_removed`、未完成的服务调用按原参数走完，so that 重载不留半配置状态。
6. As D, I want 换设备型号只新增/替换 profile YAML，so that 换型号不碰代码。
7. As D, I want 启动时校验端口冲突（不占用 8080/1700），so that 不与 ChirpStack 抢端口。

### 采集与上报

8. As C, I want 按周期采集并上报（快照 + 增量），so that 云端有基线也能看变化。
9. As C, I want 每个属性带 `quality` 与真实采样 `ts`，so that 旧值不会被当成新值。
10. As C, I want 设备离线/恢复时收到事件与一帧全量快照，so that 断档不用云端自己补。
11. As C, I want 一条 485 总线多从站时采集不互相拖死，so that 一台离线传感器不影响其他设备。
12. As C, I want 阀门故障/恢复正常时收到事件，so that 我能驱动告警。

### 服务调用

13. As C, I want 下行用 `service_id + seq`，so that 我能标识一次请求。
14. As C, I want 同 `seq` 重发不重复执行，so that 网络抖动不会让阀门动作两次。
15. As C, I want 回执区分「网关已发射 / 设备已收到 / 业务已完成」，so that LoRa 不会把发出当成功。
16. As C, I want 链路故障时回 `5040` 而不是超时，so that 我能区分「我们自己断了」和「设备不理我」。
17. As C, I want 连续下发多个开度指令时最终只执行最新值，so that 阀门不做无意义中间动作。
18. As C, I want 参数越界被拒绝并回 `4001` 带字段名，so that 我能在云端尽早发现配置错。
19. As C, I want 重启后终态未知的请求被明确拒绝（`4004`），so that 我不依赖边缘猜测。

### 运维与联调

20. As O, I want JSON 结构化日志（device/point/seq/link/reason），so that 我 grep 就能定位。
21. As O, I want 日志脱敏（不打印密码/token/payload 原文），so that 排障不泄露凭据。
22. As O, I want 可选的只读健康端点绑 `127.0.0.1:9100`（`/healthz`、`/status`、`/metrics`），so that 现场探活不登设备。
23. As O, I want 断网期间事件有界缓存、恢复后先事件后快照，so that 短暂断网不丢关键事件也不写满磁盘。
24. As O, I want 缓存满时丢最旧事件并记 `event_dropped`，so that 丢弃是可观测的。
25. As R, I want `simulate` 一条命令起 Modbus 从站 + ChirpStack 桩 + MQTT broker，so that 无实物也能跑通全链路。
26. As R, I want `snapshot --device <sn>` 一次性读当前点位，so that LoRa 驱动不会被误当轮询入口。
27. As R, I want `queue flush --device <sn>` 运维子命令（二次确认），so that 现场能手工清队列。
28. As R, I want 内核测试用假驱动/假通道/假时钟/内存 store，so that 逻辑验证不依赖硬件与真实时间。

### 交付

29. As D, I want 交付文档包含部署、Topic、字段、Modbus 映射、测试说明，so that 我能从零装起来。
30. As D, I want `Makefile` 交叉编译 `linux/arm64` 与 `amd64`（`CGO_ENABLED=0`），so that 产物可直接上设备。

## Implementation Decisions

### 1. 配置模型（四层，字段形状如下）

> 所有键名以本文件为准；GLOSSARY 的 Point 字段记为 `address|dev_eui`，配置里实际落在 `address` 块内。

#### `config.yaml`（节点级）

```yaml
node:
  id: gateway-001                 # 必填，全局唯一
  product_key: irrigation         # 必填，Topic 前缀用

northbound:
  mqtt:
    broker: mqtt://cloud.example.com:1883   # 必填；mqtt|mqtts|ssl
    client_id: "collector-{node.id}"        # 模板，支持 {node.id}
    username: ""
    password: ""                            # 或 env:COLLECTOR_MQTT_PASSWORD
    qos: 1                                  # 0|1；v1 不支持 2
    keepalive_s: 60
    tls: {enabled: false, ca_file: "", insecure_skip_verify: false}
    reconnect: {min_backoff_s: 1, max_backoff_s: 30}

adapter:
  name: irrigation-v1
  topics:                                   # 模板可改，不得在代码里硬编码
    property_post: "v1/{product_key}/{device_sn}/property/post"
    service_reply: "v1/{product_key}/{device_sn}/service/reply"
    event: "v1/{product_key}/{device_sn}/event/{event_id}"
    service_invoke: "v1/{product_key}/{device_sn}/service/invoke"
    node_state: "v1/{product_key}/$nodes/{node_id}/state"

timing:
  snapshot_interval_s: 60        # 全量上报周期
  delta_debounce_ms: 2000        # 变化合并窗口
  freshness_window_s: 180        # 超过则 quality=stale；建议 ≥ 3×采样周期
  offline_after_intervals: 3     # 倍数：Modbus=轮询周期，LoRa=期望上报间隔
  read_deadline_ms: 300          # 单设备单次读超时
  write_verify_deadline_ms: 500  # 写后回读的等待上限
  cmd_timeout_ms: 600000         # 服务调用默认超时（LoRa 会按窗口数覆盖）
  cmd_retry_max: 3               # 仅对允许重发的驱动生效
  lora_ack_windows: 2            # D6：等待的上报窗口数
  lora_window_margin_s: 30

cache:
  event_queue_max: 1000
  event_max_bytes: 2048
  event_dropped_reserve: 1         # 预留槽，保证 event_dropped 自己不被丢

queue:
  pending_max: 4                   # 每设备 pending 下行上限（D22）

state:
  db_path: /var/lib/collect-code/state.db
  idempotency_ttl_hours: 168
  compaction_interval_s: 3600

log:
  level: info                    # debug|info|warn|error
  format: json
  redact_fields: [password, token, authorization, data]

health:
  enabled: false                 # 生产部署文档要求开启
  addr: 127.0.0.1:9100
```

#### `links.yaml`（南向链路，设备按 `id` 引用）

```yaml
links:
  - id: sensors-485              # 必填，唯一；devices.yaml 的 link 引用它
    driver: modbus-rtu
    serial: {device: /dev/ttyS3, baud: 9600, data_bits: 8, parity: none, stop_bits: 1}
    read_deadline_ms: 300         # 覆盖 timing 默认值
    retry: {max: 2, backoff_ms: 100}

  - id: valves-net
    driver: modbus-tcp
    tcp: {host: 192.168.1.50, port: 502}
    read_deadline_ms: 500
    retry: {max: 2, backoff_ms: 100}

  - id: sensors-485-tcp        # 串口服务器：RTU 帧 over TCP（决策 5=A）
    driver: modbus-rtu-tcp
    tcp: {host: 192.168.1.60, port: 4001}
    read_deadline_ms: 300
    retry: {max: 2, backoff_ms: 100}

  - id: lora-gw
    driver: lorawan
    chirpstack:
      grpc_addr: 127.0.0.1:8080
      api_token_env: CHIRPSTACK_TOKEN
      application_id: "00000000-0000-0000-0000-000000000000"
      mqtt: {broker: mqtt://127.0.0.1:1883, topic_filter: "application/+/device/+/event/+", json: true}
      expected_uplink_interval_s: 300
      enqueue_timeout_s: 5
```

#### `profiles/*.yaml`（设备模板：点位 + 服务 + 事件）

```yaml
# valve-modbus.yaml
profile: valve-modbus
device_type: valve              # valve|sensor
poll_interval_s: 5
points:
  - key: opening
    access: r
    modbus: {func: 3, address: 1, type: uint16, scale: 0.1, unit: "%"}
    stale_after_s: 30
  - key: target_opening
    access: rw
    modbus: {func: 3, address: 2, type: uint16, scale: 0.1, unit: "%", min: 0, max: 100}
    write: {func: 6, address: 2}          # 写用哪个功能码/地址
  - key: state_word
    access: r
    modbus: {func: 3, address: 0, type: uint16}
  - key: fault_code
    access: r
    modbus: {func: 3, address: 9, type: uint16}

services:
  - service_id: set_opening
    confirmed: true                       # LoRa 是否用 confirmed 下行（Modbus 忽略）
    params:
      opening: {type: number, min: 0, max: 100, unit: "%"}
    writes:
      - point: target_opening
        from: params.opening
        verify: {point: opening, tolerance: 1.0}   # 写后回读校验
    timeout_ms: 600000                    # 可选，覆盖 timing.cmd_timeout_ms
    retry_max: 3                          # 仅在驱动允许重发时生效

events:
  - event_id: valve_fault
    trigger: {type: point_changed, point: fault_code, op: ne, value: 0}
    level: error
```

```yaml
# sensor-soil.yaml
profile: sensor-soil
device_type: sensor
poll_interval_s: 60
points:
  - {key: soil_temperature, access: r, modbus: {func: 4, address: 0, type: int16,  scale: 0.1,  unit: "℃"}}
  - {key: soil_moisture,    access: r, modbus: {func: 4, address: 1, type: uint16, scale: 0.1,  unit: "%"}}
  - {key: soil_ec,          access: r, modbus: {func: 4, address: 2, type: uint16, scale: 1,    unit: "µS/cm"}}
  - {key: soil_ph,          access: r, modbus: {func: 4, address: 3, type: uint16, scale: 0.01, unit: "pH"}}
```

```yaml
# valve-lora.yaml
profile: valve-lora
device_type: valve
payload:
  f_port: 10
  decode:                                       # 声明式位域（D28）
    - {key: opening,        offset: 0, length: 2, type: uint16, scale: 0.1, unit: "%"}
    - {key: state_word,     offset: 2, length: 1, type: uint8}
    - {key: fault_code,     offset: 3, length: 1, type: uint8}
    - {key: battery,        offset: 4, length: 1, type: uint8, unit: "%"}
  encode:
    - {key: target_opening, offset: 0, length: 2, type: uint16, scale: 0.1}
services:
  - service_id: set_opening
    confirmed: true
    params: {opening: {type: number, min: 0, max: 100, unit: "%"}}
    writes: [{point: target_opening, from: params.opening}]
    verify: {type: business_ack, point: target_opening, tolerance: 1.0}
    timeout_ms: 600000
    retry_max: 3
events:
  - {event_id: valve_fault, trigger: {type: point_changed, point: fault_code, op: ne, value: 0}, level: error}
```

#### 热重载矩阵（SIGHUP 的唯一口径）

| 配置块 | 可热重载 | 说明 |
|---|---|---|
| `devices.yaml` | 是 | 增删设备、改站号、改周期；移除设备发一次 `device_removed` |
| `profiles/*.yaml` | 是 | 点位/服务/事件变更；正在执行的调用按旧快照走完 |
| `links.yaml` | **否** | 链路增删/传输参数变更需重启 |
| `timing.*` | 是 | 采集/上报/新鲜度/离线参数，下一次 tick 生效 |
| `northbound.mqtt.*` | **否** | 需重启（连接与凭据重建） |
| `adapter.topics` | **否** | 需重启（订阅过滤器变化） |
| `node.*` / `state.db_path` / `health.addr` | **否** | 需重启 |

#### `devices.yaml`（实例）

```yaml
devices:
  - sn: SENSOR-0001
    device_type: sensor
    profile: sensor-soil
    link: sensors-485
    address: {unit_id: 1}
    poll_interval_s: 60            # 可选，覆盖 profile
  - sn: VALVE-0001
    device_type: valve
    profile: valve-modbus
    link: valves-net
    address: {unit_id: 1}
  - sn: VALVE-LORA-0001
    device_type: valve
    profile: valve-lora
    link: lora-gw
    address: {dev_eui: "0000000000000001", f_port: 10}
```

**周期归属**：`poll_interval_s` 是**设备**的采集周期（覆盖 profile 默认值）；链路的 tick 取该链路所有设备周期的最大公约数的下限（至少 1 s），每次 tick 只处理「到期」的设备。上报周期 `snapshot_interval_s` 是**节点级**，与采集周期解耦。

### 2. 配置校验规则（`config-check` 的验收依据）

- 必填：`node.id`、`node.product_key`、`northbound.mqtt.broker`、每条 link 的 `id/driver/传输块`、每个 device 的 `sn/device_type/profile/link/address`。
- 唯一：link `id`、device `sn`、profile 内 point `key`、service `service_id`、event `event_id`。
- 引用完整：device.profile 文件存在；device.link 存在；service.writes[].point 在 profile.points 中且 `access=rw`；verify.point 存在且可读。
- 数值：`min ≤ max`；`scale > 0`；`poll_interval_s ≥ 1`；`freshness_window_s > 0`；`offline_after_intervals ≥ 1`；`qos ∈ {0,1}`。
- 互斥：`parity` 枚举合法；`driver=lorawan` 的 link 必须有 chirpstack 块；`driver=modbus-rtu` 必须有 serial 块且 `modbus-tcp` 必须有 tcp 块。
- 端口：`health.addr` 不得与 `lora-gw.chirpstack.grpc_addr` 相同（默认 8080/1700 禁用）。

### 3. 接口契约（含 Clock 与 Store 两条缝隙）

```go
// 缝隙一：协议与传输
type Driver interface {
    Name() string
    Open(ctx context.Context) error
    Close() error
    Capabilities() Capabilities
    Read(ctx context.Context, req ReadRequest) ([]Sample, Outcome)
    Write(ctx context.Context, req WriteRequest) (CommandHandle, error)
    Events() <-chan DriverEvent   // 推送型；拉取型返回 nil
    Status() LinkStatus
}
type Capabilities struct{ Pollable, SyncVerify, AsyncAck, Confirmed bool }

type Sample struct {
    PointKey string
    Value    any
    Quality  Quality     // 驱动只给 bad，或 good 初值；最终由内核判
    At       time.Time   // 必须来自注入的 Clock
    Err      string
}
type Outcome int   // OutcomeOK | OutcomePartial | OutcomeFailed | OutcomeUnsupported
type CommandState int // CmdAccepted | CmdDelivered | CmdCompleted | CmdFailed
type DriverEvent struct {
    Kind   EventKind // uplink | txack | ack | device_status | driver_error
    Device string
    Ref    string
    State  CommandState
    Sample []Sample
    Payload []byte
    At     time.Time
}

// 缝隙二：持久化（ADR-0004 §4a）
type Store interface {
    BeginAccepted(ctx context.Context, rec AcceptedRecord) error
    Finish(ctx context.Context, rec FinalRecord) error
    Lookup(ctx context.Context, device string, seq uint64) (Record, bool, error)
    LastSeq(ctx context.Context, device string) (uint64, error)
    LoadDevices(ctx context.Context) ([]DeviceState, error)
    SaveDevices(ctx context.Context, states []DeviceState) error
    LoadSnapshot(ctx context.Context, device string) ([]Property, bool, error)
    SaveSnapshot(ctx context.Context, device string, props []Property) error
    DeleteExpired(ctx context.Context, before time.Time) (int, error)
    Close() error
}

// 时间（ADR-0003 决策 5）
type Clock interface {
    Now() time.Time
    NewTicker(d time.Duration) Ticker
    NewTimer(d time.Duration) Timer
    Sleep(d time.Duration)
}
```

- `Channel`：`Publish/Subscribe/Status`；`Message{Topic, Payload, QoS, Retain, DedupKey}`。
- `UplinkAdapter`：`PropertyReport/EventReport/DecodePayload`。
- `DownlinkAdapter`：`ParseInvocation/ServiceReply`。
- 装配只在 `cmd/collect-code`；`internal/core/*` 不 import 任何具体驱动/通道/适配器。
- bbolt 实现放在 `internal/store/bbolt`，**不在** `internal/core/*` 下；core 只 import `interfaces.Store`。

### 4. 报文契约（内联，ticket 直接引用）

**Topic**（模板见 config）：

| 方向 | 模板 | 说明 |
|---|---|---|
| 上行 | `v1/{product_key}/{device_sn}/property/post` | 属性 |
| 上行 | `v1/{product_key}/{device_sn}/service/reply` | 服务回执 |
| 上行 | `v1/{product_key}/{device_sn}/event/{event_id}` | 事件 |
| 下行 | `v1/{product_key}/{device_sn}/service/invoke` | 服务调用 |
| 上行 | `v1/{product_key}/$nodes/{node_id}/state` | 节点级 LWT，`{"online":false,"node_id":"..."}` |

**属性上报**：

```json
{"msg_id":"01JCH7...","type":"property.post","mode":"snapshot",
 "product_key":"irrigation","device_sn":"VALVE-0001","device_type":"valve",
 "ts":1763400000123,"online":true,
 "properties":[{"key":"opening","value":42.5,"unit":"%","quality":"good","ts":1763400000000}]}
```

- `mode`: `snapshot`|`delta`；`quality`: `good`|`stale`|`bad` 必填；属性 `ts` 是采样时刻。

**服务调用 / 回执**：

```json
// 下行
{"msg_id":"...","type":"service.invoke","device_sn":"VALVE-0001",
 "service_id":"set_opening","seq":17,"params":{"opening":50.0},
 "timeout_ms":600000,"issued_ts":1763400000000}
// 上行
{"msg_id":"...","type":"service.reply","device_sn":"VALVE-0001",
 "service_id":"set_opening","seq":17,"code":0,"msg":"ok",
 "data":{"accepted":true,"stage":"business","confirmed":true,
         "target_opening":50.0,"opening":49.8},
 "replay":false,"elapsed_ms":512300}
```

**code 表**：

| code | 含义 |
|---|---|
| 0 | 成功（业务确认 C 级已到） |
| 4001 | 参数非法（越界/类型/缺字段），`data.field` 指明字段 |
| 4002 | 设备不存在或未启用 |
| 4003 | `seq` 重复且终态已知 → 幂等回放，`replay=true` |
| 4004 | `seq` 重复但终态未知 → 拒绝，要求换新 `seq` |
| 4041 | 点位未映射 / 只读 |
| 5040 | 下发失败（链路/驱动不可用；链路 down 期间一律此码） |
| 5041 | 已发射/已送达但未获业务确认；`data.stage` 标明 `gateway_tx` 或 `device_mac` |
| 5042 | 重发耗尽仍无业务确认（仅允许重发的驱动） |
| 5043 | 写入已执行但回读值超出容差（`data.expected`/`data.actual`） |
| 5044 | 帧已发出但设备在 deadline 内无响应（设备不理我们，链路本身正常） |

**失败码判定矩阵**（实现与验收的唯一口径）：

| 驱动 | 场景 | code | 是否重发 | 事件 |
|---|---|---|---|---|
| Modbus | 链路 down / 端口不可用 | 5040 | 否 | `link_down` |
| Modbus | 帧已发、无响应（超时） | 5044 | 否（D23） | 无（计入链路失败计数） |
| Modbus | 写响应正常、回读超差 | 5043 | 否 | 无 |
| Modbus | 设备异常码（0x83+code） | 4001（参数）/5044（设备内部错） | 否 | 无 |
| LoRa | 链路/ChirpStack 不可用 | 5040 | 否 | `link_down` |
| LoRa | 仅 txack | 5041（`stage=gateway_tx`） | 是，至 `cmd_retry_max` | `cmd_timeout`（耗尽后） |
| LoRa | txack+ack，无业务确认 | 5041（`stage=device_mac`） | 是，至 `cmd_retry_max` | `cmd_timeout`（耗尽后） |
| LoRa | 重发耗尽 | 5042 | — | `cmd_timeout(reason=window_exhausted)` |
| LoRa | 业务确认到但回读超差 | 5043 | 否 | 无 |

**`data.stage`**：`gateway_tx`（A 级）｜`device_mac`（B 级）｜`business`（C 级）。

**C 级（业务确认）判定算法**（唯一口径）：在收到 A 或 B 级后，后续 uplink 解码出的 `opening` 与引擎受理时记录的 `target_opening` 满足 `abs(opening - target_opening) <= verify.tolerance`，且 `state_word` 表示「非运行中」，才置 `completed`。`txack`/`ack` 本身**永不**回 `code=0`。`target_opening` 只由引擎在受理时写入 state，**不从设备 payload 解码**。

**事件枚举**：

| event_id | 触发 |
|---|---|
| `device_offline` / `device_online` | 状态翻转 |
| `valve_fault` | `fault_code` 由 0 变非 0，或非 0 变为另一个非 0（`data.old/new`） |
| `valve_fault_clear` | `fault_code` 由非 0 变 0 |
| `cmd_timeout` | 回执窗口耗尽（`reason=window_exhausted`） |
| `cmd_rejected` | 设备侧拒绝 |
| `cmd_dropped` | 队列超限（`reason=queue_full`）或同 service 合并取代（`reason=superseded`） |
| `cmd_superseded_after_send` | 合并时旧目标已被取走/下发，无法撤销（ADR-0005） |
| `event_dropped` | 缓存满丢事件（带条数） |
| `link_down` / `link_up` | 链路状态翻转 |
| `device_removed` | 配置重载移除（一次性） |
| `driver_error` | 驱动异常（状态倒退、协议错） |
| `config_rejected` | SIGHUP 解析失败 |

**事件去重与标识**：一个「事件实例」由 `{device_sn}:{event_id}:{episode}` 标识，`episode` 在每次状态翻转时递增；同一实例的重发复用同一 `msg_id` 与 `dedup_key`。状态再次翻转即新实例，不会被永久抑制。云端按 `dedup_key` 去重，文档写明这一责任。

### 5.1 质量、新鲜度与在线判定（唯一真值表）

| 输入 | `last_success_at` 推进？ | 最终 quality |
|---|---|---|
| `OutcomeOK` 且至少一个样本 `At` 在窗内且驱动未给 bad | 是 | 该样本按 `now-At > effective_stale_after` 判 good/stale |
| `OutcomePartial` 且其中至少一个样本满足上式 | 是（仅按新鲜样本） | 新鲜样本 good/stale；失败点 bad |
| 全部样本 `At` 超窗 | **否** | stale（`ts` 不前移） |
| `OutcomeFailed` | 否 | 全部 bad |
| `OutcomeUnsupported`（推送型无新观测） | 否 | 保持原值与原 quality |
| 非法 payload / 解码失败 | 否 | bad |
| A 级 txack / B 级 ack | 否 | 不改变属性 |
| 业务 uplink 且解码成功 | 是（按 uplink 的 `At`） | 同第一行 |

- `effective_stale_after = point.stale_after_s`，未配置时取 `timing.freshness_window_s`。
- 最终 quality **只由内核计算**；Channel 只序列化，Adapter 不得改写。
- 离线：连续 `offline_after_intervals` 个「设备周期」内没有推进过 `last_success_at` → offline；恢复即发 online + 全量快照。
- 边界判定统一用 `>`（严格大于），测试覆盖 `window-1ns` / `window` / `window+1ns`。

### 5. 可靠性语义（参数化）

| 项 | 规则 | 参数 |
|---|---|---|
| 幂等 | `(device,seq)` 至多一次受理；`accepted→final` 两阶段 | `idempotency_ttl_hours=168` |
| 重试 | 同 `seq` 不重复执行；重试须换新 `seq` | 契约，非参数 |
| Modbus 写 | **一次 + 写后回读**，不重发 | `write_verify_deadline_ms=500` |
| LoRa 重发 | 允许，仅当写语义确认幂等 | `cmd_retry_max=3` |
| LoRa 超时 | 按**上报窗口数**，不按固定秒 | `lora_ack_windows=2`、`lora_window_margin_s=30` |
| 分级确认 | 仅 C 级回 0；A/B 级回 5041 | — |
| 链路故障 | 回 5040，不产 `cmd_timeout` | — |
| 队列合并 | 同 device+service **始终**只留最新 | `pending_max=4` |
| 缓存 | 事件 1000 条×≤2 KB；属性仅最新快照；恢复先事件后快照 | `event_queue_max=1000` |
| 离线判定 | 连续 `offline_after_intervals` 个周期无新鲜观测 | `3` |
| 新鲜度 | `now - Sample.At > effective_stale_after` → `stale` | `point.stale_after_s ?? 180` |
| 上行去重 | 用 ChirpStack `deduplicationId`；缺失时用 `devEui+fCnt`；仅内存窗口，重启不保证 | 窗口 1024 |

### 5.2 健康与可观测接口

| 端点 | 语义 | 验收 |
|---|---|---|
| `/healthz` | 仅表示进程存活，恒 200 | 进程在即 200；不反映下游状态 |
| `/status` | JSON：store 可用性、MQTT 连接状态、各链路状态、每设备在线状态、队列深度、计数器 | schema 固定；与内部状态一致 |
| `/metrics` | Prometheus 文本：`collector_link_ok{link}`、`collector_poll_total{link,result}`、`collector_queue_depth{device}`、`collector_event_dropped_total`、`collector_missed_ticks_total{link}` | 指标名/类型/标签固定，测试断言取值 |

- 默认不监听；开启仅绑 `health.addr`（默认 `127.0.0.1:9100`）。
- 启动时**只 connect 不 bind** 8080/1700；`config-check` 拒绝 `health.addr` 与之相同。

### 6. 子命令

| 子命令 | 用途 |
|---|---|
| `run` | 常驻运行 |
| `config-check` | 解析 + 校验 + 打印生效配置摘要 |
| `simulate` | 起模拟器（见下） |
| `snapshot --device <sn>` | 一次性读点位（LoRa `Read` 的唯一调用方） |
| `queue flush --device <sn>` | 运维清队列，需 `--yes` |
| `version` | 版本与构建信息 |

### 7. 模拟器（`simulate`）

一个进程内同时起：

- **Modbus TCP 从站**：多 unit_id，寄存器内存来自 profile 的 `ASSUMED` 映射（台账 B 段）。
- **Modbus RTU 从站**：通过 **RTU-over-TCP** 传输暴露（决策 5=A）。它与生产链路 `modbus-rtu-tcp` 是同一条路径，因此 CI 测的就是现场可能用的接法；串口 `modbus-rtu` 的 termios/波特率留实物联调。
- **ChirpStack 桩**：gRPC `Enqueue/GetQueue/FlushQueue` + 按 `expected_uplink_interval_s` 产生 `up` 事件 + 按队列 FIFO 产生 `txack`/`ack`；队列项 `isPending` 可查询。
- **MQTT broker + 云端桩**：进程内嵌纯 Go broker（`mochi-mqtt`，决策 6=A），监听 `127.0.0.1:0`；云端桩订阅上行、按脚本或 CLI 发下行 `service.invoke`。
- 所有模拟值来自同一份 profile/台账数据，保证「模拟的就是被测的」。
- **对端可观测接口**（验收的一部分，不是调试附属）：
  - 启动时输出 `manifest.json`（`--manifest <path>`），含各端点实际地址与随机端口。
  - `GET /_sim/captured`：收到的上行报文 `{topic,payload,qos,retain,received_at}`。
  - `GET /_sim/requests`：Modbus 请求日志 `{unit_id,function,address,quantity,raw_bytes,at}`。
  - `POST /_sim/control`：`unit_offline`/`unit_slow`/`unit_bad_crc`/`link_stop`/`broker_disconnect`/`queue_full`，支持按 unit 粒度。
  - `POST /_sim/invoke`：注入完整下行报文。
- 从站必须独立实现寄存器语义（站号路由、功能码、地址、数量、CRC、异常码），测试用**独立 Modbus 客户端**验证，不复用被测驱动。

## Testing Decisions

### 缝隙与分层（承认两条内核缝隙）

| 层 | 范围 | 时间 | 依赖 | 放哪 |
|---|---|---|---|---|
| L1 单元 | 内核组件（scheduler/state/invoker/forward/eventbus） | **假 Clock** | 假 Driver/Channel/Adapter + 内存 Store | CI 每 PR |
| L2 集成 | 驱动编解码、store 两阶段、适配器、通道重连 | 假 Clock（编解码可无时间） | 内存对端 / 真实 bbolt 临时目录 / ChirpStack 桩 | CI 每 PR |
| L3 端到端 | 真实二进制 + `simulate` + 内嵌 broker | **真实时钟** | 进程/端口 | nightly 30–60 min |
| L4 发布门禁 | 真实二进制 + 真实时钟 | 真实 24 h | 专用 runner | v1.0 发布前人工 |

> **修正**：假 Clock 无法注入独立进程，所以 L3/L4 用真实时钟；「24h 稳定性」不在 ticket 级验收里，它是 v1.0 发布门禁（决策 7=A）。

### 假 Clock 的可行性

- 内核构造时注入 `Clock`；`internal/core/*` 与 `internal/drivers/*` 禁止直接调用 `time.Now/NewTicker/NewTimer/After/AfterFunc/Sleep`；业务超时不用 `context.WithTimeout`（运行时定时器不可快进），context 只做取消传播。
- 该规则由 **CI 静态检查**执行（`scripts/check-time.sh`：`grep -rn` 或自定义 `go/ast` 分析器），**不接受 code review 清单**。
- 测试提供 `FakeClock`（`Now` 可设、`Advance` 推进、触发所有到期 ticker/timer）。所有 `Sample.At` 由同一 FakeClock 产出。
- 可确定性验证：相位不累积、missed_tick 跳过、单设备 deadline 放弃、服务超时窗口、缓存 TTL。

### bbolt 崩溃恢复怎么造

- 不用「写一半」或随机 kill。bbolt 事务原子，崩溃窗口在**两次事务之间**。
- 测试：`BeginAccepted` → `Close`（不调 `Finish`）→ 重开 → 断言 `Lookup` 返回 `phase=accepted` → invoker 对该 `seq` 回 `4004` 且不重放。
- 耐久性（fsync）用子进程自执行模式：写 accepted 后 `os.Exit(0)`，父进程重开断言。放 nightly，不进 PR。

### 模拟器与 CI

- RTU 从站走 RTU-over-TCP，零外部依赖；与生产 `modbus-rtu-tcp` 链路共用实现。
- 内嵌 MQTT broker（纯 Go，`mochi-mqtt`）；`simulate` 自包含，不依赖 docker/apt。
- ChirpStack 桩是「迷你队列模拟器」，不是空壳：必须模拟 FIFO、`isPending`、`txack`/`ack` 的时序，否则 F10 的三种回执组合测不出来。

### 稳定性验收的数值判据

- **L1（假时钟快进 24 h，CI）**：60 s 周期 = 1440 tick；断言上报条数 = 1440 ± 2、最大相邻间隔 ≤ 1.2×period、无 `device_offline`、无 `bad/stale`、tick 计划时刻不累积漂移。
- **L3（真实时钟 30–60 min，nightly）**：真实 socket/重连；断言无进程崩溃、无未处理错误日志、断线后能补发。
- **L4（真实 24 h，v1.0 门禁）**：人工/runner 执行，同上判据 + 内存与 `state.db` 增长在预算内（≤100 MB / ≤50 MB）。
- 真实抖动 ≤ ±10% 只在 L3/L4 判；L1 只证「不累积」。

### v0.1 独立性的前提

- 接口（`Capabilities`/`CommandState`/`EventKind`）在 v0.1 就按全量定义，LoRa 只需填不同的能力布尔与事件，不改接口。
- **唯一会返工的是 `state`**：新鲜观测必须写成 **Read 与 Event 双入口**，`offline_after`/`freshness_window` 全程配置驱动，不得按 Modbus 周期硬编码。v0.1 就要这么写。

## Out of Scope

（同 `docs/产品定型.md` §3 N1–N9）

- 云端后台、数据库；改 ChirpStack 源码；硬件驱动；固件 OTA。
- 本地 Web UI；多节点编排；云端配置下发。
- 定时灌溉策略、轮灌计划、规则引擎。
- 主备双接入路径自动切换与对账；一机一密、远程凭据轮换。
- LoRa payload 的压缩/条件/变长编码（v1 只支持声明式位域）。
- MQTT QoS 2；TLS 双向认证（v1 支持单向 + CA）。

## Further Notes

- 版本路线：v0.1 485→MQTT；v0.2 Modbus TCP 阀门 + 服务调用 + 幂等；v0.3 LoRa 上下行 + 分级确认 + 队列治理；v0.4 可靠性/运维；v1.0 实物联调 + 文档交付。
- `docs/缺口台账.md` 中 `ASSUMED` 项在真实资料到位后只替换配置与测试夹具，不改内核。
- 交付文档五章：部署（含 systemd/端口/日志）、Topic 与字段（含 `seq` 契约与分级确认）、Modbus 映射表、测试说明（含模拟器）、排障（含 `config-check`/`queue flush`/`event_dropped`）。**各 ticket 增量维护自己的章节**；19 只做索引、doc contract 与可执行 smoke。
- 验收的通用判据：每条 ticket 的验收项必须能被一个确定的测试或可复现脚本判定；否定句（「不重发」「不调用」「不丢」）必须有可观测载体（计数器/调用日志/capture API）。
