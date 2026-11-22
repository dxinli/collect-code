# 02b: Store 契约与 bbolt 实现

**What to build:** 幂等状态与设备快照能崩溃安全地落盘，且内存实现与 bbolt 实现行为一致。

**Blocked by:** 02a

**Status:** ready-for-agent

- [ ] 同一套契约测试分别跑内存实现与 bbolt 实现
- [ ] 覆盖 BeginAccepted/Finish/重复 Begin/Finish 未知记录/LastSeq/LoadDevices/SaveDevices/LoadSnapshot/SaveSnapshot/DeleteExpired
- [ ] 两阶段：BeginAccepted → Close（不 Finish）→ 重开 → Lookup 仍为 accepted
- [ ] Close/重开后数据一致；并发写不 panic；bbolt 实现位于 `internal/store/bbolt`

## Comments

- 2026-11-22 plan 已并入 main；实现暂停，等待 `docs/spec/spec.md` v3 与 `docs/contracts/store.md`。
- 解冻时：merge main → 用户拍板 plan §4.5/§4.12/§4.13 的 A/B → 合并 `spec-delta/02b-store` → 按 store 合同实现，不再修改 spec/contract。
- 2026-11-23 用户确认 plan §4 全部按推荐 A，并明确“计划评审通过”；`spec-delta/02b-store` 已合并（规范包 3.1.0）。
- 实施约束：只按 `docs/contracts/store.md` 实现；实现提交不得修改 spec/contract；完成后更新 `docs/traceability.md` 与 ticket。
- 2026-11-23 实现完成，PR #12（`feat/02b-store-bbolt`）。共享契约套件 37 项对 memory/bbolt 双实现全绿；`-race` 与并发用例 `-count=10` 全绿；`make ci` 全绿。`docs/traceability.md` 的 `REQ-SEAM-02` 与 8 个 `STORE-*` 已改为 `covered`。等待用户验收后再勾选验收项并标 `done`。
