# ADR-0002：ChirpStack v4 的接入边界——上行走 MQTT integration，下行走 gRPC

- 状态：Accepted
- 日期：2026-11-17
- 相关：`docs/产品定型.md` D22/D25、`docs/缺口台账.md` A2–A6
- 事实来源：已核实的官方 proto / 文档 / 源码（见文末证据）

## 背景

需求要求「打通 LoRa 上行、下行；确认下行调用接口；网关数据上云链路保留」。ChipStack 是既有系统，本服务只能作为旁路消费者。

核实过程中推翻了两条常见错误假设：

1. **REST API 不在 8080，也不是主进程内置。** v4 已移除内嵌 REST；`POST /api/devices/{queueItem.devEui}/queue` 由独立组件 `chirpstack-rest-api` 提供，默认监听 **8090**，转发到 gRPC `localhost:8080`。网关现场很可能**只装了主进程、没装 rest-api**。
2. **没有「无损上行 API」。** `ApplicationService` 没有 stream 方法；唯一的事件流是 `InternalService.StreamDeviceEvents`，它读 Redis Stream，受 `per_device_event_log_max_history`（默认 10 条）与 TTL（默认 31 天）限制，**是事件日志，不是完整上行通道**。

同时确认：

- 鉴权统一为 `Authorization: Bearer <token>`，token 是 JWT，作用域为 admin 或 tenant；v3 token 在 v4 不可用。
- MQTT integration 默认事件 topic：`application/{{application_id}}/device/{{dev_eui}}/event/{{event}}`，事件类型 `up / status / join / ack / txack / log / location / integration`；JSON 模式下 `data` 是 **base64**。
- enqueue 请求体字段：`queueItem{ id, confirmed, fPort, data(base64), object, isPending, fCntDown, isEncrypted, expiresAt }`，**没有 priority 字段**。
- 队列治理接口存在：`GET …/queue`（GetQueue）、`DELETE …/queue`（FlushQueue）。
- Class A 权威表述：*"Only after an uplink transmission by the device, ChirpStack is able to schedule a downlink transmission."* 队列项持久化，收到上行触发的接收窗口后按 FIFO 下发，**mac-commands 优先于应用 payload**。
- 官方 Go 客户端：`github.com/chirpstack/chirpstack/api/go/v4`，标签形如 `api/go/v4.20.0`。

## 决策

1. **上行只用 MQTT integration**（引擎作为只读 MQTT 客户端订阅 `application/+/device/+/event/+`）。它是官方对外集成路径、无丢帧历史限制、断线重连语义简单。
2. **下行用 gRPC 客户端直接调 `DeviceService/Enqueue`**，不使用 REST。理由：不要求现场额外安装 `chirpstack-rest-api`；官方 Go 客户端开箱即用；`FlushQueue`、`GetQueue` 同一客户端可达。
3. **不采用 `InternalService.StreamDeviceEvents` 作为上行主通道**：其历史深度默认 10 条，重启或断线期间的帧会丢，不足以承载采集数据。
4. **队列治理用 `FlushQueue` + `GetQueue` 实现 D22**：同一设备的 pending 下行按「同 service 合并、保留最新」，超限淘汰最旧并发 `cmd_dropped`。
5. **`confirmed` 字段按需设置**：默认 `false`。confirmed 下行会占用设备接收窗口并影响功耗，只在确有必要的服务上开启。
6. ChirpStack 配置只允许**增量启用 integration**（属需求允许的「配置、集成」范围），不修改其源码、不占用 8080/1700。

## 后果

- 部署前置条件：ChirpStack 侧须存在一个 **MQTT integration**（broker 地址、topic 前缀、JSON 编码）。若现场不允许启用，降级路径按序为：Redis Streams（需 Redis 可达）→ `InternalService.StreamDeviceEvents`（明确接受丢帧，日志告警）。降级路径写入部署文档。
- 上行 payload 解码需处理 base64（不是 hex），且 `internal.rs` 的 `LogItem.properties["Data"]` 是 hex —— 两处编码不同，**不要混用**。
- 下行时延敏感：ChirpStack v4.8.1 起可在 device-profile 调大 RX1 delay 以适应集成链路延迟。若现场时延大导致错过 RX1/RX2，应在 device-profile 侧调参（属对方配置，文档说明即可）。
- 队列深度与过期（`expiresAt`，v4.10.1+）需在部署文档中说明；旧版本无 `expiresAt`，过期只能由我方淘汰策略保证。

## 被否决的方案

- **REST-only 下行**：依赖现场额外安装 `chirpstack-rest-api`（默认 8090），多一个组件与故障点。
- **`InternalService.StreamDeviceEvents` 作上行**：默认仅保留 10 条事件，重启/断线必丢帧。
- **直连 Redis Streams 作主上行**：需 Redis 凭据与网络可达，耦合 ChirpStack 内部实现细节，版本升级易断。
- **依赖 `state_topic`（retained 状态）**：文档模板里有该配置，但 `config.rs`/`integration/mqtt.rs` 在 master 与各 v4 tag 均无此字段，**代码未实现**，不可依赖。

## 证据

- `chirpstack/api/proto/api/device.proto`（Enqueue/FlushQueue/GetQueue、queueItem 字段、confirmed）
- `chirpstack-rest-api/main.go`、`openapiv2/api/device.swagger.json`（8090、路径参数）
- `chirpstack/src/api/auth/mod.rs`（Bearer / JWT 校验）
- `chirpstack/src/integration/mqtt.rs`、`src/config.rs`（默认 topic、7 种事件、无 state_topic）
- `api/proto/api/internal.proto`（InternalService stream、无 ApplicationService stream）
- docs：`integrations/mqtt.html`、`integrations/events.html`、`features/device-classes.html`、`configuration.html`、`changelog.html`
- Go 模块：`api/go/go.mod`，`api/go/v4.20.0`
