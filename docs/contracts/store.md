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
- 记录损坏时按“损坏记录”节返回错误，不得当作正常记录返回。

## STORE-LASTSEQ-01 `LastSeq`

- 返回每设备已受理 `seq` 的单调高水位。
- 初始值为 0；不得回退；只由 `BeginAccepted` 推进。
- 该水位由已受理的 `seq` 物化，不依赖记录是否仍存在；清理过期记录不得降低该水位。

## STORE-DEVICES-01 `LoadDevices` / `SaveDevices`

- `LoadDevices` 在空库返回空切片与 nil 错误；返回顺序不作保证，调用方不得依赖顺序。
- `SaveDevices` 按设备 upsert；不得删除未出现在入参中的设备。设备移除由内核的状态协调逻辑负责。
- `SaveDevices` 只更新设备状态记录，不得推进或修改幂等水位；`DeviceState.LastSeq` 与 `Store.LastSeq` 是两份数据，其一致性由内核协调，Store 不做隐式同步。
- 持久化的设备状态至少包含在线状态、最后成功时间、最后 `seq` 与更新时间。

## STORE-SNAPSHOT-01 `LoadSnapshot` / `SaveSnapshot`

- `LoadSnapshot` 对未知设备返回 `found=false`；对已知设备返回属性副本，调用方修改返回值不得影响存储。
- `LoadSnapshot` 保留保存时的属性顺序。
- `Property.Quality` 是不透明字符串；Store 不校验、不转换，原样往返。
- `SaveSnapshot(nil)` 等价于保存空快照；随后 `LoadSnapshot` 返回 `found=true` 与非 nil 空切片，而不是 `found=false`。
- `SaveSnapshot` 必须原子替换单个设备的快照；不得让读者看到半更新的快照。
- 快照只保留每设备最新值，不保留历史序列。

## STORE-EXPIRY-01 `DeleteExpired`

- 只删除 `final` 且终态时刻早于截止时间的记录。
- 必须返回删除条数。
- 不得删除 `accepted` 记录；不得降低 `LastSeq` 水位。

## STORE-CLOSE-01 `Close`

- `Close` 必须幂等：首次成功后，再次 `Close` 返回 nil。
- `Close` 之后，所有带 ctx 的方法必须返回 `ErrClosed`，不得再访问底层 DB 或内部数据。
- `Close` 与在途调用并发时不得 panic 或死锁；实现必须用锁保护关闭状态。

## 持久化值语义

- `Data` 与 `Property.Value` 经 JSON 语义往返：数字解码为 `float64`；`nil` 保持 `nil`；嵌套对象/数组为 `map[string]any`/`[]any`；`Data` 内的 `time.Time` 变为 RFC3339 字符串。
- 时间型字段（`AcceptedAt`、`FinalAt`、`LastSuccessAt`、`UpdatedAt`、`At`）按时刻比较；零值 `time.Time{}` 可往返。
- 调用方不得用 `reflect.DeepEqual` 或 `==` 比较往返后的值；必须按 JSON 语义或时刻比较。
- 绝对值超过 `2^53` 的整数经 `any` 往返会丢精度；需要在 `Data`/`Value` 中保留大整数时必须先转为字符串。

## 输入前置条件

- `device` 必须非空；空 `device` 不在本合同范围内，Store 不承诺跨实现一致的错误。
- `seq` 由云端分配；Store 不额外校验 `seq` 的业务单调性，只按 `(device, seq)` 存取。

## 损坏记录

- 无法按合同解码的记录、或 `phase` 不是 `accepted`/`final` 的记录，视为损坏。
- 读取路径遇到损坏记录必须返回错误，不得当作正常记录返回。
- `DeleteExpired` 遇到损坏记录必须使整个事务回滚，不得部分删除或跳过。
- 内存实现没有公开路径产生损坏记录；其行为不作为跨实现一致性要求。
