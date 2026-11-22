# Store 契约：持久化语义

本文件定义内核持久化缝隙的规范语义。**接口签名与冻结注释以 `internal/interfaces/store.go` 为准**；本文件不复制签名，只定义调用方可以依赖的行为。行为背景见 ADR-0004 及其 Amendment。

生产实现只允许嵌入式 KV，bbolt 实现必须位于 `internal/store/bbolt`，不得位于内核包。测试内存实现必须通过与生产实现相同的合同测试。

所有方法都必须接受上下文，并在上下文结束后及时返回。除特别说明外，未知记录不是错误：返回零值与 `found=false`。

## STORE-ACCEPT-01 `BeginAccepted`

- 在产生外部副作用之前写入一条 `(device, seq)` 的 `accepted` 记录。
- 若该键已存在（无论 `accepted` 还是 `final`），必须返回“已受理”错误，且不得覆盖原记录。
- 记录必须包含设备、`seq`、服务标识与受理时刻。

## STORE-FINISH-01 `Finish`

- 将已存在的 `accepted` 记录推进为 `final`，写入 code、消息、数据与终态时刻。
- 未知记录必须返回“记录不存在”错误。
- 已经是 `final` 的记录必须返回“已终态”错误，不得覆盖首次终态。

## STORE-LOOKUP-01 `Lookup`

- 未知键返回 `found=false` 与 nil 错误。
- 已知键返回完整记录；调用方据 `phase` 区分“受理未完成”与“已有终态”。
- `phase=accepted` 表示终态未知；重启后对这类键必须回 `4004`，不得重放。

## STORE-LASTSEQ-01 `LastSeq`

- 返回每设备已受理 `seq` 的单调高水位。
- 初始值为 0；不得回退；只由 `BeginAccepted` 推进。
- 该水位由已受理的 `seq` 物化，不依赖记录是否仍存在；清理过期记录不得降低该水位。

## STORE-DEVICES-01 `LoadDevices` / `SaveDevices`

- `LoadDevices` 在空库返回空切片与 nil 错误。
- `SaveDevices` 按设备 upsert；不得删除未出现在入参中的设备。设备移除由内核的状态协调逻辑负责。
- `SaveDevices` 只更新设备状态记录，不得推进或修改幂等水位；`DeviceState.LastSeq` 与 `Store.LastSeq` 是两份数据，其一致性由内核协调，Store 不做隐式同步。
- 持久化的设备状态至少包含在线状态、最后成功时间、最后 `seq` 与更新时间。

## STORE-SNAPSHOT-01 `LoadSnapshot` / `SaveSnapshot`

- `LoadSnapshot` 对未知设备返回 `found=false`；对已知设备返回属性副本，调用方修改返回值不得影响存储。
- `SaveSnapshot` 必须原子替换单个设备的快照；不得让读者看到半更新的快照。
- 快照只保留每设备最新值，不保留历史序列。

## STORE-EXPIRY-01 `DeleteExpired`

- 只删除 `final` 且终态时刻早于截止时间的记录。
- 必须返回删除条数。
- 不得删除 `accepted` 记录；不得降低 `LastSeq` 水位。

## STORE-CLOSE-01 `Close`

- 关闭后资源必须释放；再次关闭的语义以冻结合同测试为准。
- 进行中的调用在关闭后的行为必须在 02b 批准后固定，调用方不得提前依赖未批准语义。

## 02b 待批准语义（pending）

以下条目属于 02b 实现补充，**尚未批准**。在 `spec-delta/02b-store` 合并前，调用方不得依赖这些行为；本文件也不在其中选择方案。

| 条目 | 状态 |
|---|---|
| JSON 往返语义：持久化值经过 JSON 语义，整数可能变为浮点数；时间按时刻比较；禁止用 `reflect.DeepEqual` 或 `==` 比较往返结果 | pending 02b plan approval |
| 超过 2^53 的整数精度边界与替代表示 | pending 02b plan approval |
| 设备列表与快照属性的返回顺序 | pending 02b plan approval |
| `Close` 后各方法的错误语义与幂等性 | pending 02b plan approval |
| `SaveSnapshot` 传入空切片时表示“空快照”还是“无操作” | pending 02b plan approval |
| 空设备标识的前置条件与错误 | pending 02b plan approval |
| 损坏记录的处理策略 | pending 02b plan approval |

02b 的解冻流程：用户先对 02b 计划中的未决选择拍板；从 main 开 `spec-delta/02b-store` 更新本文件与版本/变更记录；评审合并后，02b 才可继续实现。实现提交不得再修改本文件。02a/02b 计划是来源记录，不是新工作的必读权威。
