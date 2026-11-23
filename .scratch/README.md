# Plan archive

New ticket implementation plans live in the ticket PR body. This directory
stores diff notes, historical plans, and deferred plans only. Do not add a new
active plan here; open an issue and put the plan in its PR body.

Do not move the files below: old issues and PRs link to these paths.

| Plan | Status | Location |
|---|---|---|
| 01a 工程骨架 | implemented | `collector/plans/01a.md` |
| 01b 配置校验 | implemented | `collector/plans/01b.md` |
| 01c 配置语义 | implemented | `collector/plans/01c.md` |
| 02a 内核接口与假时钟 | implemented | `collector/plans/02a.md` |
| 02b Store 与 bbolt | implemented | `collector/plans/02b.md` |
| 03a 调度相位 | implemented | `collector/plans/03a.md` |
| 04a Modbus PDU | implemented | `collector/plans/04a.md` |
| 05a MQTT 通道 | implemented | `collector/plans/05a.md` |
| 06b 内嵌 broker | implemented | `collector/plans/06b.md` |
| spec v3 迁移 | implemented | `collector/plans/spec-v3.md` |
| 06b diff 说明 | historical | `collector/diffs/06b.md` |
| spec v3 diff 说明 | historical | `collector/diffs/spec-v3.md` |
| GitHub Issues 迁移 | implemented (PR #58) | `github-issues/plans/01-migration.md` |
| main 分支保护 | deferred (ADR-0007) | `main-protection/plans/01-policy.md` |

Historical files may contain section-sign references from the v2-draft spec.
They are marked at the top and are intentionally frozen; current references
must use `REQ-*` or contract anchors.
