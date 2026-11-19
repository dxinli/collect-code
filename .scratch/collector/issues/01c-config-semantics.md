# 01c: 配置语义补强（01b 移出的 4 条规则）

**What to build:** 补齐四类“配置写得出来但运行期必坏”的语义检查；这些规则不在 spec §2 明写，01b 计划评审决定移出，单独成单。

**Blocked by:** 01b

**Status:** backlog

- [ ] `device.device_type` 与 `profile.device_type` 都非空时必须一致；不新增 profile.device_type 必填
- [ ] `device_type` 枚举限定 `valve`/`sensor`/`gateway`（以 `GLOSSARY.md` 为准）
- [ ] `event.trigger.type == "point_changed"` 时，`trigger.point` 必须在 profile 点位集合中且可读；沿用 01b P1=A 的 `points ∪ payload.decode(读) ∪ payload.encode(写)` 口径
- [ ] `service.writes[].from` 必须形如 `params.<name>`，且 `<name>` 已在 `service.params` 声明
- [ ] 每条规则一个合法用例 + 一个非法用例；规则号从 V70 起，不占用 01b 的 V01–V63
- [ ] 更新 spec §2 校验规则；如摘要需要展示新字段，再更新摘要与 golden
- [ ] 不实现运行时行为；只做 `config-check` 校验

## Comments

- 2026-11-19 由 01b 计划评审移出。01b 只做规则目录 V01–V63；本 ticket 不阻塞 01b。
- 建议规则号：V70 device_type 一致性、V71 device_type 枚举、V72 trigger.point、V73 writes[].from。
