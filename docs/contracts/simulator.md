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
| `--capture-capacity <n>` | `1000` | 上行捕获环形缓冲容量，范围 `1..1000000` |
| `--topic-filter <filter>` | `#` | 云端桩唯一的 MQTT 订阅过滤器 |
| `-h`, `--help` | — | 帮助写 stdout 并退出 0 |

退出码：`0` 成功或收到 SIGINT/SIGTERM 优雅退出；`1` 启动或运行错误；`2` 用法错误。成功运行时 stdout 必须为空；结构化 JSON 日志写 stderr，至少包含 `event=startup|shutdown`。

06b 的模拟器不加载生产配置，也没有 `--config`。它是纯传输对端，不假设硬件或协议。需要 profile 数据的后续模块必须新增可选 flag 接入，不得改变既有 flag 语义。

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
  "endpoints": {"captured": "/_sim/captured", "invoke": "/_sim/invoke", "control": "/_sim/control"},
  "control_actions": {"global": ["broker_disconnect"]},
  "modules": ["mqtt"]
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

## SIM-CONTROL-01 `POST /_sim/control`

请求信封为 `{"module": "<optional>", "action": "<required>", ...}`。无 `module` 的动作归入 manifest 的 `control_actions.global`。

06b 只注册 `broker_disconnect`：

```json
{"action": "broker_disconnect", "duration_ms": 5000}
```

- 必须踢掉 broker 上所有已连接的非 inline 客户端：关闭 TCP，不发送 DISCONNECT。
- `duration_ms` 可选，默认 `0`，范围 `0..86400000`。大于 0 时进入保持期：保持期内新 CONNECT 必须被拒绝（MQTT 3.1.1 CONNACK code 5）。
- 保持期必须惰性自动恢复：每次 CONNECT 用注入时钟比较 `hold_until`，到期即接受；不得要求第二个控制动作。
- listener 必须始终绑定原端口，manifest 的 `mqtt.url` 不变；重复调用覆盖 `hold_until` 并再次踢当前连接。
- 成功响应为 `{"ok":true,"action":"broker_disconnect","disconnected":N,"hold_until":"...|null"}`。
- 未知 module/action 或 `duration_ms` 越界返回 400；动作处理器必须严格解码专有字段，未知字段 400。

## SIM-SECURITY-01 安全与测试

- 默认只绑 loopback。显式传入非 loopback 地址时必须启动日志告警，但不阻止。无鉴权、无 TLS，仅面向本机测试。
- broker 的 hook 默认拒绝连接与 ACL；实现必须用单个 hook 同时提供连接认证（保持期拒绝，其余放行）与 ACL 检查（恒放行）。
- 测试必须使用仓库内独立的 MQTT 3.1.1 客户端（直接读写 TCP 字节，不 import broker 库）验证连接、订阅、发布、捕获、注入、断线与恢复；不得要求 docker、apt 或 mosquitto。
- 后续模块必须在同一进程、同一 `/_sim/*` HTTP 服务、同一 manifest 上追加，不得改变本文件的既有契约。

## 模块与非目标

当前 06b 模块只装配 MQTT broker 与云端桩。后续模块：

| 模块 | 内容 | 约束 |
|---|---|---|
| 06a | Modbus TCP / RTU-over-TCP 从站 | 寄存器语义独立实现；测试用独立客户端；追加 `/_sim/requests` |
| 12a/12b | ChirpStack gRPC 队列与 MQTT 事件桩 | 模拟 FIFO、`isPending`、`txack`/`ack` 时序；不占用 8080/1700 |

当前版本非目标：

- 不提供 `GET /_sim/requests`（属 06a）。
- 不提供 ChirpStack MQTT fixture（属 12b）。
- 不提供 `--config`，不加载生产配置。
- 不提供生产 MQTT 北向通道（属 05a）。
- 不实现压缩、条件或变长 payload 编码。

模拟值要求：06a 的寄存器值与 12b 的 ChirpStack 值必须来自与被测系统相同的 profile 与缺口台账数据，保证“模拟的就是被测的”。06b 是纯传输层，没有“模拟值”，自包含不构成冲突；需要 profile 时由后续模块新增可选 flag，不返工。
