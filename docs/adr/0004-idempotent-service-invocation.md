# ADR-0004：服务调用用「至多一次受理 + 幂等去重 + 分级确认」，幂等与状态走嵌入式 KV（bbolt）

- 状态：Accepted
- 日期：2026-11-17
- 相关：`docs/产品定型.md` D3/D4/D20/D21/D22/D23/D26/D27、ADR-0005、`docs/缺口台账.md` B12/E6、`AGENTS.md` 硬约束

## 背景

对阀门的误动作是**物理后果**，不是数据错误。同时边缘节点 `kill -9` 与断电都很常见。

三个必须同时满足的约束：

1. 云端重发同一条服务调用（网络抖动、超时后用户重试）不能导致阀门动作两次。
2. 进程在「已受理、尚未执行」处崩溃，重启后不能重复执行。
3. Class A 设备离线期间不能无限重发把 ChirpStack 下行队列塞满（官方 FIFO 下发，队列满会丢帧）。

持久化形态已由用户确认（决策 4=B，台账 E6）：**允许嵌入式 KV**。选用 `go.etcd.io/bbolt`——纯 Go、无 cgo、与 `CGO_ENABLED=0` 静态编译兼容、单文件、ACID 事务。**不用 sqlite**：其 Go 绑定依赖 cgo，破坏静态编译硬约束。

## 决策

1. **受理语义是「至多一次受理」**：`invoker` 收到一条服务调用，先做幂等判定；同一 `(device, seq)` 已存在则**不再下发**，按存储里的终态返回。首次受理才向下推进。
2. **幂等键是 `(device_sn, seq)`**，`seq` 为 **uint64**、按设备单调递增、由**云端**分配。边缘不做自增编号，只做窗口内去重。
3. **幂等状态是 bbolt 里一条记录的两阶段状态**（不再用 append-only journal）：
   - 事务 1（提交即 `fsync`）：写入 `{device, seq, service_id, ts, phase=accepted}`，然后才调用驱动 `Write`。
   - 事务 2：更新同一条记录 `{code, summary, phase=final}`。
   - 崩溃窗口仍落在「`accepted` 已提交、副作用未发生」或「副作用已发生、`final` 未提交」两侧。**前者是安全的**（少执行）；后者导致终态未知——重启后对该 `seq` 回 **`4004`（终态未知，请换新 `seq`）**，而不是猜测性重放。
   - 说明：bbolt 让「写这条记录」本身成为原子、崩溃安全的单次事务，消除了 journal 的 append/replay/压缩机制，**但它无法消除上面这个窗口**——因为副作用发生在外部（网线/射频上），存储事务性不覆盖它。所以 `accepted/final` 两阶段语义保留，只是落地形式从「两行日志」变成「一条记录的状态字段」。
4. **过期清理**：后台周期扫描 `phase=final` 且 `ts` 早于 `idempotency_ttl`（默认 7 天）的记录并删除，同时物化每设备 `last_seq` 水位。设备在线状态与属性快照存同一 bbolt 文件的不同 bucket。

4a. **存储是内核的第二条缝隙**：`core/store` 暴露窄接口，bbolt 是唯一生产实现，测试另提供内存实现。

   ```go
   type Store interface {
       BeginAccepted(ctx context.Context, rec AcceptedRecord) error
       Finish(ctx context.Context, rec FinalRecord) error
       Lookup(ctx context.Context, device string, seq uint64) (Record, bool, error)
       LastSeq(ctx context.Context, device string) (uint64, error)
       LoadDevices(ctx context.Context) ([]DeviceState, error)
       SaveDevices(ctx context.Context, states []DeviceState) error
       Close() error
   }
   ```

   内核对 bbolt 无直接依赖；`internal/core/store` 的 bbolt 实现自己测两阶段与重开，其余内核用内存实现。**因此「内核只有一条缝隙」的说法不再成立——是两条：`interfaces`（协议与传输）与 `store`（持久化）。**

5. **云端契约**（必须写进对接文档）：
   - 重试**必须换新 `seq`**；同 `seq` 只用于「同一条请求的重发」，边缘不会重复执行。
   - 重试窗口必须 **小于幂等 TTL**（7 天），否则过期后同 `seq` 会被当成新请求再执行一次。
   - `seq` 需保证 64 位空间内单调递增、不回绕。
6. **分级确认**：`CmdAccepted → CmdDelivered → CmdCompleted`，或任意态 → `CmdFailed`。对应 LoRa 的 A 级（`txack`，网关已发射）/ B 级（`ack`，设备 MAC 确认，仅 `confirmed` 下行）/ C 级（业务 ACK）。只有 `CmdCompleted`（C 级）才回 `code=0`；到 A 或 B 级但未见业务确认 → `code=5041` 并在 `data.stage` 标明级别。
7. **重发策略按驱动能力分流**：
   - `Capabilities.SyncVerify=true`（Modbus）：**不重发**，改为「一次写入 + 写后回读验证」。因为厂商寄存器可能是「写即动作」，重发等于让阀门反复动作（台账 B12 未确认前必须保守）。
   - `SyncVerify=false` 且 `AsyncAck=true`（LoRa），且客户确认写语义幂等：允许重发，上限 3 次，每次间隔按**上报窗口数**计（D6），且受第 8 条（队列治理）约束。
8. **下行队列治理**（D22）：同设备同 `service_id` 的 pending 下行**始终合并、只保留最新**；每设备 pending 上限 4，超限淘汰最旧并记 `cmd_dropped(reason=queue_full)`。合并的实现用 `GetQueue` + 逐条精确撤销，**不用 `FlushQueue`**（竞态与最坏后果见 ADR-0005）。

## 后果

- 幂等表可重建：bbolt 是崩溃安全的事务存储，`state.db` 损坏概率远低于整文件 JSON 重写；即使损坏，幂等表只影响「去重」，不影响「重复执行」的安全侧（丢失记录会让旧 `seq` 变成新请求，与 TTL 过期同语义）。
- **终态已知才回放，未知则拒执行**：`4003`（回放终态）与 `4004`（终态未知，换新 `seq`）是两个不同语义，不能合并成一个码。
- **崩溃后可能「少执行」**：`accepted` 已提交而 `final` 未提交时，重启后该 `seq` 不会被重放，而是回 `4004` 要求换新 `seq`。这是有意的取舍——宁可少动作，不可多动作。
- 写放大受控：每条受理两次 bbolt 事务，量级等于服务调用频率（不是采集频率），在网关上可接受。
- **「至多一次受理」不保证执行成功**，只保证不重复。执行成功靠分级确认与重发（仅 LoRa 且已确认写幂等的场景）。
- `seq` 窗口：云端保证 64 位空间内单调且不回绕。若边缘收到明显回退的 `seq`（远小于窗口下界），拒绝并回 `4001`，不做猜测性执行。

## 被否决的方案

- **纯内存幂等**：重启即失效，正是最危险的时刻。
- **先执行后落盘**：崩溃窗口落在「副作用已发生」，重启会重复动作。
- **append-only journal + 周期截断**：在 bbolt 可用后属于重复造轮子；journal 的 append/replay/压缩都要自己维护，且崩溃恢复逻辑更容易写错。
- **sqlite**：Go 绑定依赖 cgo，与 `CGO_ENABLED=0` 静态编译冲突（AGENTS.md 硬约束）。
- **Modbus 也做「至少一次重发」**：在写入语义未确认前有物理风险。
- **不限队列的 LoRa 重发**：Class A 离线时队列塞满，设备上线先执行最旧指令。

## Amendment（2026-11-22）

本 ADR 记录决策与理由；**规范正文**已迁移为：

- 至多一次受理、两阶段状态、分级确认：[`REQ-INVOKE-01..05`](../spec/spec.md)
- 队列合并与缓存：[`REQ-QUEUE-01..03`、`REQ-CACHE-01..02`](../spec/spec.md)
- Store 规范语义：[`docs/contracts/store.md`](../contracts/store.md)
- 外部 code / seq / stage：[`docs/contracts/external.md`](../contracts/external.md)

决策 4a 中的接口片段是当时的七个方法。当前冻结签名以 `internal/interfaces/store.go` 为准（已含 `LoadSnapshot`、`SaveSnapshot`、`DeleteExpired` 与 `Close`）；bbolt 实现位于 `internal/store/bbolt`，不在 `internal/core/store`。本文不改写，以避免历史被追溯性修改。
