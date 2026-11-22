# 02b: Store 契约与 bbolt 实现

**What to build:** 幂等状态与设备快照能崩溃安全地落盘，且内存实现与 bbolt 实现行为一致。

**Blocked by:** 02a

**Status:** on-hold

- [ ] 同一套契约测试分别跑内存实现与 bbolt 实现
- [ ] 覆盖 BeginAccepted/Finish/重复 Begin/Finish 未知记录/LastSeq/LoadDevices/SaveDevices/LoadSnapshot/SaveSnapshot/DeleteExpired
- [ ] 两阶段：BeginAccepted → Close（不 Finish）→ 重开 → Lookup 仍为 accepted
- [ ] Close/重开后数据一致；并发写不 panic；bbolt 实现位于 `internal/store/bbolt`

## Comments

- 2026-11-22 plan 已并入 main；实现暂停，等待 spec v3 / `docs/contracts/store.md`。
- 解冻时：merge main → 引用规范化 → 按 contract 实现，不再修改 spec。
