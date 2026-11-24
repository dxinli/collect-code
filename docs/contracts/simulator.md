# 模拟器契约

本文件定义 `collect-code simulate` 的 CLI、manifest 与 HTTP API。行为要求见 `REQ-SIM-01`；本文件是模拟器的规范来源。

模拟器是**测试对端**，不是被测实现的一部分。它必须能被真实二进制连接，并让测试观察、注入与控制传输行为。

## SIM-CLI-01 命令行

`collect-code simulate` 的 flag：

| Flag | 默认 | 语义 |
|---|---|---|
| `--manifest <path>` | 空（不写） | 启动快照输出路径；目标目录必须已存在，不自动创建 |
| `--http-addr <host:port>` | `127.0.0.1:0` | 云端桩 HTTP 地址；端口 0 表示随机 |
| `--mqtt-addr <host:port>` | `127.0.0.1:0` | 内嵌 broker 监听地址；端口 0 表示随机 |
| `--modbus-addr <host:port>` | `127.0.0.1:0` | Modbus TCP 从站地址；端口 0 表示随机 |
| `--modbus-rtu-addr <host:port>` | `127.0.0.1:0` | RTU-over-TCP 从站地址；端口 0 表示随机 |
| `--config <dir>` | 空（内置 ASSUMED 表） | 可选；加载与被测系统相同的四层配置，派生 Modbus unit 与寄存器映射；加载或校验失败为启动错误（退出码 1） |
| `--capture-capacity <n>` | `1000` | 上行捕获环形缓冲容量，范围 `1..1000000` |
| `--topic-filter <filter>` | `#` | 云端桩唯一的 MQTT 订阅过滤器 |
| `-h`, `--help` | — | 帮助写 stdout 并退出 0 |

退出码：`0` 成功或收到 SIGINT/SIGTERM 优雅退出；`1` 启动或运行错误；`2` 用法错误。成功运行时 stdout 必须为空；结构化 JSON 日志写 stderr，至少包含 `event=startup|shutdown`。

`simulate` 总是启动 Modbus TCP 与 RTU-over-TCP 两个从站监听器（见 `SIM-MODBUS-01`），端口 0 表示由内核分配。06b 的 MQTT/HTTP flag 语义不变；`--config` 只影响 Modbus 模块，不提供时使用内置 ASSUMED 表。需要 profile 数据的后续模块必须新增可选 flag 接入，不得改变既有 flag 语义。

## SIM-MANIFEST-01 manifest

启动时可写出一份 JSON 快照，供测试发现实际地址。`schema_version` 为整数。

```json
{
  "schema_version": 1,
  "generated_at": "2026-11-20T11:22:33.123456789Z",
  "pid": 4242,
  "http": {"base_url": "http://127.0.0.1:39001", "host": "127.0.0.1", "port": 39001},
  "mqtt": {
    "url": "mqtt://127.0.0.1:39002", "host": "127.0.0.1", "port": 39002,
    "tls": false, "protocols": ["3.1.1", "5.0"],
    "topic_filters": ["#"], "capture_capacity": 1000
  },
  "modbus": {
    "tcp": {"url": "tcp://127.0.0.1:39003", "host": "127.0.0.1", "port": 39003},
    "rtu_over_tcp": {"url": "tcp://127.0.0.1:39004", "host": "127.0.0.1", "port": 39004},
    "units": [1, 2],
    "register_source": "builtin"
  },
  "endpoints": {
    "captured": "/_sim/captured", "invoke": "/_sim/invoke",
    "control": "/_sim/control", "requests": "/_sim/requests"
  },
  "control_actions": {
    "global": ["broker_disconnect"],
    "modbus": ["unit_bad_crc", "unit_offline", "unit_slow"]
  },
  "modules": ["modbus", "mqtt"]
}
```

扩展规则：

- 加法扩展不升版：新增顶层 section（如 `modbus`、`chirpstack`）、`endpoints` 新键、`control_actions` 新模块、`modules` 新元素都不破坏既有消费者。
- 删除字段或改变字段语义才升到 `2`，并必须另行评审。
- 消费者按字段名读取并忽略未知字段；测试不得断言完整键集合或逐字节 golden（端口、时间、pid 天然变化）。
- `endpoints` 名称、section 名称与 `(module, action)` 必须唯一；重复注册是启动错误。
- `http`、`schema_version`、`generated_at`、`pid`、`endpoints`、`control_actions`、`modules` 是保留 section 名。
- manifest 必须通过“同目录临时文件 + fsync + rename”原子替换。
- `mqtt.url` 可以直接作为北向 MQTT broker 地址；`http.base_url` 与 `endpoints` 拼出 HTTP 地址。
- `modbus.tcp` 与 `modbus.rtu_over_tcp` 是 06a 的模块 section；`units` 为已服务站号升序；`register_source` 为 `builtin`（内置 ASSUMED 表）或 `config`（`--config` 派生）。

## SIM-CAPTURE-01 `GET /_sim/captured`

```json
{"ok": true, "messages": [
  {"seq": 7, "topic": "v1/irrigation/V1/property/post",
   "payload": {"msg_id": "01J...", "type": "property.post"},
   "qos": 1, "retain": false, "received_at": "2026-11-20T11:22:40.001234567Z"}
], "dropped": 0}
```

- `messages` 按到达顺序；`seq` 从 1 单调递增；空时必须为 `[]`。
- `payload` 是原始字节的 JSON 表示：合法 JSON 返回 JSON 值，否则退化为 JSON 字符串。
- 捕获必须跳过 `$SYS/` 前缀与由 `/_sim/invoke` 产生的 inline-origin 消息，只表示“收到的上行”。
- 固定容量环形缓冲：满时丢最旧并累加 `dropped`，绝不阻塞 broker；响应始终返回 `dropped`。

## SIM-INVOKE-01 `POST /_sim/invoke`

```json
{"topic": "v1/irrigation/VALVE-0001/service/invoke",
 "payload": {"msg_id": "m1", "type": "service.invoke", "service_id": "set_opening", "seq": 17},
 "qos": 1, "retain": false}
```

- `topic` 必填，必须是合法 publish topic：不含 `+` 或 `#`，不以 `$SYS` 开头；空串单独判 400。
- `payload` 必填，为任意 JSON 值；必须按该值的原始字节发布，不做 `service.invoke` 语义校验，测试可以注入畸形报文。
- `qos` 可选，默认 `1`，只允许 `0` 或 `1`；`retain` 可选，默认 `false`。
- 成功响应为 `{"ok":true,"topic":...,"qos":...,"retain":...,"bytes":N}`；解析或校验错误 400，发布失败 500；未知路径 404，方法不对 405。

## SIM-MODBUS-01 Modbus 从站

`simulate` 必须在同一进程内提供 Modbus TCP（MBAP）与 RTU-over-TCP（RTU ADU）两个从站监听器。两者共享同一份 unit 状态与请求日志。

- 监听器默认 `127.0.0.1:0`；实际地址写入 manifest 的 `modbus` section。
- 站号 `unit_id` 为 `1..247`。未配置的 unit 不响应；请求记录为 `no_response`，连接保持。
- 功能码：03 读保持寄存器、04 读输入寄存器、06 写单寄存器、16 写多寄存器。
- 请求校验：
  - 03/04：quantity 必须为 `1..125`；quantity 为 0 或大于 125 返回异常 `03`；地址越界、未实现或只读返回异常 `02`；读取跨越任何非法地址时整帧拒绝，不返回部分数据。
  - 06：地址必须可写；只读或未实现返回异常 `02`。
  - 16：quantity 必须为 `1..123`；byte count 必须等于 `2 × quantity`，否则返回异常 `03`；任一地址非法返回异常 `02`；写必须原子，失败时不得部分生效。
- 异常码：`01` 非法功能、`02` 非法数据地址、`03` 非法数据值。异常响应为 `func|0x80` + code。
- Modbus TCP：MBAP 协议号必须为 0；长度必须为 `2..254`。完整 ADU 消费后的 PDU 错误按异常响应；头部非法或截断时关闭连接。
- RTU-over-TCP：使用 CRC-16/MODBUS（反射多项式 `0xA001`，初值 `0xFFFF`），对 `unit + PDU` 计算，低字节先发。请求 CRC 错误时不响应、保持连接并记录 `bad_frame`；未知功能码无法可靠确定帧边界时关闭连接并记录 `bad_frame`。
- 寄存器表与写语义以 `docs/缺口台账.md` 的模拟寄存器表与模拟初值为准；模拟器提供原始 uint16 值，缩放由引擎负责。至少覆盖示例 profile 的 `valve-modbus` 与 `sensor-soil` 点位：
  - 保持寄存器：`0x0000` 状态字、`0x0001` 当前开度、`0x0002` 目标开度（RW）、`0x0003` 阀门指令（W）、`0x0009` 故障码、`0x000A` 通讯超时计数、`0x000B` 固件版本；
  - 输入寄存器：`0x0000` 土壤温度、`0x0001` 土壤湿度、`0x0002` 土壤 EC、`0x0003` 土壤 pH、`0x0004` 介质温度、`0x0005` 管道压力、`0x0006–0x0007` 瞬时流量（uint32 大端）、`0x000C–0x000D` 累计流量（uint32 大端）。
  - 32 位值按高字在前、跨两个寄存器；int16 使用补码。
  - 写 `0x0002` 必须使 `target = current = value` 且状态字为非运行中；写 `0x0003` 的 `1`/`2`/`0` 分别表示全开/全关/不变。写多寄存器逐点套用同一语义。
  - 初值必须来自缺口台账的 `ASSUMED` 模拟初值，不得在实现里另编一套。
- 独立实现：模拟器不得 import `internal/drivers` 或 `internal/core`；MBAP、RTU、CRC 与 PDU 校验必须在 `internal/sim` 内独立实现。测试必须使用同样不 import 被测驱动的独立客户端。
- `--config`：从 `modbus-tcp` / `modbus-rtu-tcp` 链路的设备与 profile 派生 unit 和寄存器映射；同 unit 同 `(func,address)` 的类型或访问冲突、缺失 `unit_id`、配置加载失败均为启动错误。`modbus-rtu`（真实串口）设备跳过。
- 请求日志见 `SIM-REQUESTS-01`。

## SIM-REQUESTS-01 `GET /_sim/requests`

```json
{"ok": true, "requests": [
  {"seq": 1, "transport": "tcp", "remote": "127.0.0.1:49231",
   "unit_id": 1, "function": 3, "address": 1, "quantity": 2,
   "outcome": "ok", "exception": null,
   "received_at": "2026-11-24T10:00:00.000000001Z",
   "responded_at": "2026-11-24T10:00:00.000000002Z", "duration_ms": 0}
], "dropped": 0}
```

- `requests` 按到达顺序；`seq` 从 1 单调递增；空时必须为 `[]`。
- `transport` 为 `tcp` 或 `rtu_over_tcp`。`remote` 为对端地址。
- 写请求带 `values`（uint16 数组）；读请求不带。无法解析的帧可带 `raw_hex`，`unit_id` / `function` 可为 `null`。
- `outcome` ∈ `ok`、`exception`、`no_response`、`bad_frame`、`bad_crc`。
  - `ok`：正常响应；
  - `exception`：返回异常响应，`exception` 为异常码；
  - `no_response`：未配置 unit、`unit_offline` 或等价的不响应；
  - `bad_frame`：请求帧非法（MBAP 头部、RTU CRC、未知功能码等）；
  - `bad_crc`：`unit_bad_crc` 故意破坏 RTU 响应 CRC。
- `received_at` 与 `responded_at` 使用注入时钟的 UTC `RFC3339Nano`；无响应时 `responded_at` 为 `null`，`duration_ms` 为 `0`。
- 固定容量 10000 的环形缓冲：满时丢最旧并累加 `dropped`；响应始终返回 `dropped`。
- 日志在请求处理完成时追加；`no_response` 在判定时立即追加；`unit_slow` 在延迟后追加，`duration_ms` 反映实际等待。
- `raw_hex` 最多保留前 512 字节的十六进制表示。
- 只接受 `GET`；未知路径 404，方法不对 405。

## SIM-CONTROL-01 `POST /_sim/control`

请求信封为 `{"module": "<optional>", "action": "<required>", ...}`。无 `module` 的动作归入 manifest 的 `control_actions.global`。

06b 注册 `broker_disconnect`：

```json
{"action": "broker_disconnect", "duration_ms": 5000}
```

- 必须踢掉 broker 上所有已连接的非 inline 客户端：关闭 TCP，不发送 DISCONNECT。
- `duration_ms` 可选，默认 `0`，范围 `0..86400000`。大于 0 时进入保持期：保持期内新 CONNECT 必须被拒绝（MQTT 3.1.1 CONNACK code 5）。
- 保持期必须惰性自动恢复：每次 CONNECT 用注入时钟比较 `hold_until`，到期即接受；不得要求第二个控制动作。
- listener 必须始终绑定原端口，manifest 的 `mqtt.url` 不变；重复调用覆盖 `hold_until` 并再次踢当前连接。
- 成功响应为 `{"ok":true,"action":"broker_disconnect","disconnected":N,"hold_until":"...|null"}`。
- 未知 module/action 或 `duration_ms` 越界返回 400；动作处理器必须严格解码专有字段，未知字段 400。

06a 注册 `modbus` 的 per-unit 故障动作：

```json
{"module":"modbus","action":"unit_offline","unit_id":1,"duration_ms":5000}
{"module":"modbus","action":"unit_slow","unit_id":1,"delay_ms":500,"duration_ms":5000}
{"module":"modbus","action":"unit_bad_crc","unit_id":1,"duration_ms":5000}
```

- `unit_id` 必填，必须是 manifest `modbus.units` 中的已服务 unit；未知 unit 返回 400。
- `duration_ms` 可选，默认 `0`，范围 `0..86400000`；`0` 表示不自动恢复（再次控制或重启进程才清除），大于 0 表示 `until = now + duration_ms`。这与 `broker_disconnect` 的 `0` 语义不同：Modbus 动作是持续故障模式，不是一次性断线。
- `unit_slow` 的 `delay_ms` 必填，范围 `1..86400000`；每个响应延迟该值。
- 每个 unit 同一时刻只有一种故障模式；新动作替换旧模式并覆盖 `until`。
- 恢复是惰性的：每次请求用注入时钟比较 `until`，到期即清除；不得要求后台 timer。
- `unit_bad_crc` 只影响 RTU-over-TCP 的响应 CRC；Modbus TCP 没有 CRC，动作仍成功，但 TCP 响应保持合法。
- 成功响应为 `{"ok":true,"action":...,"unit_id":1,"until":"...|null"}`；`unit_slow` 额外带 `delay_ms`。
- 未知 module/action、未知字段、`unit_id` 越界、`delay_ms`/`duration_ms` 越界返回 400。

## SIM-SECURITY-01 安全与测试

- 默认只绑 loopback。显式传入非 loopback 地址时必须启动日志告警，但不阻止。无鉴权、无 TLS，仅面向本机测试。该规则同时适用于 HTTP、MQTT、Modbus TCP 与 RTU-over-TCP 监听器。
- broker 的 hook 默认拒绝连接与 ACL；实现必须用单个 hook 同时提供连接认证（保持期拒绝，其余放行）与 ACL 检查（恒放行）。
- 测试必须使用仓库内独立的 MQTT 3.1.1 客户端（直接读写 TCP 字节，不 import broker 库）验证连接、订阅、发布、捕获、注入、断线与恢复；不得要求 docker、apt 或 mosquitto。
- Modbus 测试必须使用独立的 Modbus 客户端（直接读写 TCP 字节，自带 MBAP/RTU/CRC 实现，不 import `internal/drivers/modbus`）验证寄存器、异常、请求日志与故障控制。
- 后续模块必须在同一进程、同一 `/_sim/*` HTTP 服务、同一 manifest 上追加，不得改变本文件的既有契约。

## 模块与非目标

当前模块：06b 的 MQTT broker 与云端桩，06a 的 Modbus TCP / RTU-over-TCP 从站。后续模块：

| 模块 | 内容 | 约束 |
|---|---|---|
| 12a/12b | ChirpStack gRPC 队列与 MQTT 事件桩 | 模拟 FIFO、`isPending`、`txack`/`ack` 时序；不占用 8080/1700 |

当前版本非目标：

- 不提供 ChirpStack MQTT fixture（属 12b）。
- 不提供真实串口 RTU、termios 或波特率控制；06a 只提供 RTU-over-TCP。
- 不提供生产 MQTT 北向通道（属 05a）。
- 不提供请求日志的查询过滤、分页或持久化。
- 不实现压缩、条件或变长 payload 编码。

模拟值要求：06a 的寄存器值与 12b 的 ChirpStack 值必须来自与被测系统相同的 profile 与缺口台账数据，保证“模拟的就是被测的”。06b 是纯传输层，没有“模拟值”，自包含不构成冲突；需要 profile 时由后续模块新增可选 flag，不返工。
