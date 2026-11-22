# 配置契约：四层模型与校验导航

本文件只做导航。**键名与结构以 `internal/config/types.go` 为准，默认值以 `internal/config/defaults.go` 为准，校验规则与诊断文案以 `internal/config/rules.go` 与 `internal/config/validate.go` 为准。** `configs/example/` 与 `internal/config/testdata/` 只是示例与测试夹具，不是权威。

行为语义（重载、校验失败的处理）见 `REQ-OPS-01`。

## CFG-LAYERS-01 四层文件

| 层 | 文件 | 内容 | 引用关系 |
|---|---|---|---|
| 节点 | `config.yaml` | 节点标识、北向 MQTT、适配器 Topic、时限、缓存、队列、状态库、日志、健康端点 | 无 |
| 链路 | `links.yaml` | 南向链路：Modbus TCP、串口 RTU、RTU-over-TCP、LoRaWAN/ChirpStack | 无 |
| 模板 | `profiles/*.yaml` | 设备类型：点位、服务、事件、payload 声明式位域 | 无 |
| 实例 | `devices.yaml` | 设备实例：SN、类型、profile、link、地址、周期覆盖 | `profile` 引用模板；`link` 引用链路 |

`profile` 名在模板文件内定义，设备按名字引用；`link` 的 `id` 在链路文件内定义，设备按 id 引用。配置目录必须同时包含这四层。

## CFG-VALIDATION-01 校验分类

| 类别 | 规则号 | 内容 |
|---|---|---|
| 语法与文件 | V01–V07 | YAML 语法、空文档、未知键、重复键、类型不匹配、文件缺失、profile 重名 |
| 必填 | V10–V16 | 节点、链路、设备、profile 名、点位 key、service_id、event_id |
| 唯一 | V20–V24 | 链路 id、设备 sn、profile 内 point/service/event 标识 |
| 引用完整 | V30–V35 | profile/link 引用；写入点存在且可写；校验点存在且可读 |
| 数值 | V40–V45 | `min ≤ max`、`scale > 0`、采集周期、新鲜度窗口、离线倍数、QoS |
| 枚举与传输 | V50–V53 | driver、transport 块匹配、parity、access |
| 端口与运行项 | V60–V63 | 健康端点端口冲突、凭据、日志级别、日志格式 |
| 语义补强 | V70–V73 | device_type 一致性与枚举、事件触发点、`writes[].from` 参数引用 |

**CFG-V73** `service.writes[].from` 必须写成 `params.<name>`，且 `<name>` 必须已在同一服务的 `params` 中声明；空串、缺少前缀或空名字都非法。

点位能力口径：可写集合为 profile 的 `points` 中 `access=rw` 的点位，加上 `payload.encode`；可读集合为 `points` 中 `access=r` 或 `rw` 的点位，加上 `payload.decode`。引用校验与事件触发点校验都使用这一并集口径。

诊断必须定位到引用字段所在文件的行列；格式与规则号由 `internal/config/rules.go` 定义。`config-check` 必须在存在任一错误时以非零码退出，并按文件与规则号输出全部诊断。

## CFG-RELOAD-01 热重载矩阵

| 配置块 | 可热重载 | 说明 |
|---|---|---|
| `devices.yaml` | 是 | 增删设备、改站号、改周期；移除设备上报一次移除事件 |
| `profiles/*.yaml` | 是 | 点位、服务、事件变更；进行中的调用按旧快照走完 |
| `timing.*` | 是 | 下一次调度 tick 生效 |
| `links.yaml` | 否 | 链路增删或传输参数变更需重启 |
| `northbound.mqtt.*` | 否 | 连接与凭据需重建 |
| `adapter.topics` | 否 | 订阅过滤器与发布 Topic 变化需重启 |
| `node.*`、`state.db_path`、`health.*` | 否 | 需重启 |

解析或校验失败时整体不应用，旧配置继续运行。重载不得留下半配置状态；进行中的服务调用与断线缓存不得因重载丢失。行为要求见 `REQ-OPS-01`。
