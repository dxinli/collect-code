# 01c: 配置语义补强（01b 移出的 4 条规则）

> **历史文档（historical）**：文中的 `spec §N` 是 v2-draft 的历史引用；当前行为规范见 `docs/spec/spec.md`，配置合同见 `docs/contracts/config.md`。

<!-- spec-refs: historical-file -->

**What to build:** 补齐四类“配置写得出来但运行期必坏”的语义检查；这些规则不在 spec §2 明写，01b 计划评审决定移出，单独成单。 <!-- historical -->

**Blocked by:** 01b

**Status:** done

- [x] `device.device_type` 与 `profile.device_type` 都非空时必须一致；不新增 profile.device_type 必填
- [x] `device_type` 枚举限定 `valve`/`sensor`/`gateway`（以 `GLOSSARY.md` 为准）
- [x] `event.trigger.type == "point_changed"` 时，`trigger.point` 必须在 profile 点位集合中且可读；沿用 01b P1=A 的 `points ∪ payload.decode(读) ∪ payload.encode(写)` 口径
- [x] `service.writes[].from` 必须形如 `params.<name>`，且 `<name>` 已在 `service.params` 声明
- [x] 每条规则一个合法用例 + 一个非法用例；规则号从 V70 起，不占用 01b 的 V01–V63
- [x] 更新 spec §2 校验规则；如摘要需要展示新字段，再更新摘要与 golden <!-- historical -->
- [x] 不实现运行时行为；只做 `config-check` 校验

## Comments

- 2026-11-19 由 01b 计划评审移出。01b 只做规则目录 V01–V63；本 ticket 不阻塞 01b。
- 建议规则号：V70 device_type 一致性、V71 device_type 枚举、V72 trigger.point、V73 writes[].from。
- 2026-11-19 计划 v6 经用户明确“计划评审通过”；决策 1B（V71 同时校验 profile）、2B（空 `trigger.point` 用专用 required 消息）、3B（`params.<name>` 按 map key 精确查找，允许已声明的带点名称）。
- 实现提交：`059662a` spec、`d8eef27` v33 夹具、`358effa` 点位能力重构、`857cc97` V70–V73、`c2878ee` 单测、`3e90fe8` golden。
- 规则 4 条：V70 一致性、V71 枚举、V72 trigger 点位、V73 from 参数。既有 golden 只修改 `invalid/v33-access-unset`（补 `from`，位置 `8:10`→`10:10`），摘要与 `golden/summary.stdout` 未改。
- 验证：`make ci` 全绿（fmt/print/go fix/vet/test/build/static/config-check/race/lint 0 issues）。
- 后续：完整 ticket 01（01a/01b/01c）的代码走读按 `CLAUDE.md` 单独提交。
