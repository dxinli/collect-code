# 外部契约：MQTT 报文与云端对接

本文件定义引擎与云端之间的 Topic、JSON 字段、code、阶段、事件与 `seq` 契约。行为语义见 `docs/spec/spec.md`；配置模板见 `docs/contracts/config.md`。

规范词：**必须**为强制要求，**不得**为禁止。

## EXT-TOPIC-01 Topic 模板

Topic 是适配器配置，不是代码常量。默认模板如下：

| 方向 | 模板 | 用途 |
|---|---|---|
| 上行 | `v1/{product_key}/{device_sn}/property/post` | 属性（快照/增量） |
| 上行 | `v1/{product_key}/{device_sn}/service/reply` | 服务回执 |
| 上行 | `v1/{product_key}/{device_sn}/event/{event_id}` | 事件 |
| 下行 | `v1/{product_key}/{device_sn}/service/invoke` | 服务调用 |
| 上行 | `v1/{product_key}/$nodes/{node_id}/state` | 节点级遗嘱消息 |

`product_key` 来自节点配置，`device_sn` 与 `node_id` 来自实例配置。改前缀或层级只改配置，不得要求改内核。

## EXT-PROP-01 属性上报

`mode` 必须为 `snapshot` 或 `delta`。`snapshot` 携带该设备当前全部属性，`delta` 只携带本次变化项。`quality` 必须为 `good`、`stale` 或 `bad`，且每个属性都必须带。属性 `ts` 是该值实际获得的时刻，不是发送时刻；`stale` 时不得前移。

```json
{
  "msg_id": "01JCH7...",
  "type": "property.post",
  "mode": "snapshot",
  "product_key": "irrigation",
  "device_sn": "VALVE-0001",
  "device_type": "valve",
  "ts": 1763400000123,
  "online": true,
  "properties": [
    {"key": "opening", "value": 42.5, "unit": "%", "quality": "good", "ts": 1763400000000}
  ]
}
```

字段约束：

| 字段 | 类型 | 说明 |
|---|---|---|
| `msg_id` | string | 全局唯一，用于云端幂等 |
| `type` | string | 固定 `property.post` |
| `mode` | string | `snapshot` / `delta` |
| `ts` | int64 | 本帧生成时刻，毫秒 |
| `online` | bool | 该设备当前在线状态 |
| `properties[].value` | number / bool / string | 数值按 profile 的缩放与类型解码 |
| `properties[].unit` | string / null | 无量纲时为 null |
| `properties[].ts` | int64 | 采样时刻，毫秒 |

## EXT-INVOKE-01 服务调用与回执

下行调用必须包含 `msg_id`、`type=service.invoke`、`device_sn`、`service_id`、`seq`、`params`、`timeout_ms`、`issued_ts`。上行回执必须包含 `msg_id`、`type=service.reply`、`device_sn`、`service_id`、`seq`、`code`、`msg`、`data`、`replay`、`elapsed_ms`。

```json
{"msg_id":"01JCH8...","type":"service.invoke","device_sn":"VALVE-0001",
 "service_id":"set_opening","seq":17,"params":{"opening":50.0},
 "timeout_ms":600000,"issued_ts":1763400000000}
```

```json
{"msg_id":"01JCH9...","type":"service.reply","device_sn":"VALVE-0001",
 "service_id":"set_opening","seq":17,"code":0,"msg":"ok",
 "data":{"accepted":true,"stage":"business","confirmed":true,
         "target_opening":50.0,"opening":49.8},
 "replay":false,"elapsed_ms":512300}
```

`replay=true` 只用于已知终态的幂等命中；它不是本次新执行的结果。

## EXT-SEQ-01 `seq` 契约

| 规则 | 要求 |
|---|---|
| 位宽 | `uint64` |
| 分配 | 云端分配；边缘不自增 |
| 单调性 | 按设备单调递增，64 位空间内不回绕 |
| 重试 | 同一请求的重发复用同一 `seq`；语义不同的新请求必须换新 `seq` |
| 窗口 | 云端重试窗口必须小于引擎的幂等保留期 |
| 回退 | 明显回退到窗口下界之外的 `seq` 必须拒绝，回 `4001` |

不同服务不得复用同一 `seq`。

## EXT-CODE-01 code 表

| code | 含义 |
|---|---|
| 0 | 成功；业务确认（C 级）已到 |
| 4001 | 参数非法（越界、类型错误、缺字段、`seq` 回退）；`data.field` 指明字段 |
| 4002 | 设备不存在或未启用 |
| 4003 | `seq` 重复且终态已知；幂等回放，`replay=true` |
| 4004 | `seq` 重复但终态未知；拒绝执行，要求换新 `seq` |
| 4041 | 点位未映射或只读 |
| 5040 | 下发失败：链路或驱动不可用；链路 down 期间一律此码 |
| 5041 | 已发射或已送达，但未获业务确认；`data.stage` 标明级别 |
| 5042 | 重发耗尽仍无业务确认（仅允许重发的驱动） |
| 5043 | 写入已执行但回读值超出容差；`data.expected` / `data.actual` |
| 5044 | 帧已发出但未获得有效响应：截止时间内无响应，或设备返回内部错误异常码；链路本身正常 |

`4003` 与 `4004` 语义不同，不得合并。`5043` 只由写后回读判定，不由驱动的写入调用直接返回。

## EXT-STAGE-01 阶段与 C 级判定

`data.stage` 必须为：

| 值 | 含义 |
|---|---|
| `gateway_tx` | A 级：网关已把下行帧发射出去 |
| `device_mac` | B 级：设备 MAC 层确认收到；仅 confirmed 下行有 |
| `business` | C 级：业务确认已到 |

只有 C 级可以回 `code=0`。A 或 B 级必须回 `5041`，并带对应的 `stage`。业务确认判定使用后续上行解码出的实际开度与引擎受理时记录的目标开度，且设备状态字表示非运行中；容差由服务配置。

## EXT-EVENT-01 事件

事件报文必须包含 `msg_id`、`type=event`、`device_sn`、`event_id`、`level`、`ts`、`data`、`dedup_key`。

```json
{"msg_id":"01JCHA...","type":"event","device_sn":"VALVE-0001","event_id":"cmd_timeout",
 "level":"warn","ts":1763400600000,
 "data":{"service_id":"set_opening","seq":17,"attempts":3},
 "dedup_key":"VALVE-0001:cmd_timeout:17"}
```

| event_id | 触发与 `data` |
|---|---|
| `device_offline` / `device_online` | 在线状态翻转 |
| `valve_fault` | 故障码由 0 变非 0，或由非 0 变为另一个非 0；带 `old` / `new` |
| `valve_fault_clear` | 故障码由非 0 变 0；带 `old` / `new` |
| `cmd_timeout` | 回执窗口耗尽；`reason=window_exhausted` |
| `cmd_rejected` | 设备侧拒绝 |
| `cmd_dropped` | 队列超限（`reason=queue_full`）或待下发条目被新值取代（`reason=superseded`） |
| `cmd_superseded_after_send` | 合并时旧目标已被取走或已下发，无法撤销 |
| `event_dropped` | 缓存满丢弃事件；带丢弃条数 |
| `link_down` / `link_up` | 南向链路状态翻转 |
| `device_removed` | 配置重载移除设备；一次性 |
| `driver_error` | 驱动异常：状态倒退或协议错误 |
| `config_rejected` | SIGHUP 解析或校验失败，旧配置继续运行 |

## EXT-DEDUP-01 事件实例与去重

一个事件实例由 `{device_sn}:{event_id}:{episode}` 标识。`episode` 必须在每次状态翻转时递增；同一实例的重发必须复用同一 `msg_id` 与 `dedup_key`。状态再次翻转即新实例，不得被永久抑制。云端必须按 `dedup_key` 去重。

`dedup_key` 的默认形式为 `{device_sn}:{event_id}:{episode}`；服务调用相关事件可以使用 `seq` 作为实例标识，但必须在同一设备同一事件类型内保持唯一。

## EXT-TARGET-01 `target_opening`

`target_opening` 是引擎在受理 `set_opening` 时写入状态的期望值，不是设备上报的目标。它只在本次引擎受理成功后才有 `quality=good`；未经本引擎下发的目标值不得上报。`valve_side`（进/出水侧）不属于 v1 服务参数。

## EXT-LWT-01 节点遗嘱消息

节点级遗嘱消息只表示 MQTT 连接断开：

```json
{"online": false, "node_id": "gateway-001"}
```

设备级离线不靠遗嘱消息（一条连接无法为每台设备发遗嘱），必须通过 `device_offline` 事件与属性的 `online` 字段表达。
