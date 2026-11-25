# ADR-0007：main 分支保护采用最小门禁，agent 直推约束暂缓

- 状态：Accepted
- 日期：2026-11-24
- 相关：`.scratch/main-protection/plans/01-policy.md`、ADR-0006、`AGENTS.md`

## 背景

`main` 已经要求 PR 与 `ci`、`gitleaks` 检查，但 `plan-guard` 只是提示性 workflow，不在 required checks 中。结果是 `plan-review` 期间的代码改动、以及缺少 `Closes #<issue>` 的 ticket PR 都可能被合并。

同时，`main-protection` 计划里关于 agent 直推、ruleset 与本地 hook 的完整方案尚未实施；用户决定先保持最小门禁，等文档稳定后再评估更重的约束。

## 决策

1. `main` 必须经 PR；required status checks 为 `ci`、`gitleaks`、`guard`。
2. `guard` 在 PR 带 `plan-review` 标签时只允许 `docs/`、`.scratch/`、`AGENTS.md`、`CLAUDE.md`、`GLOSSARY.md` 的改动；代码文件一律拒绝。
3. `guard` 要求 `feat/*` 与 `hotfix/*` 分支的 PR 正文包含 `Closes #<issue>`；`chore/*`、`spec-delta/*` 不强制。
4. 暂不启用 ruleset、bot 直推、本地 hook 等针对 agent 直推的硬约束。
5. 过渡窗口内 `enforce_admins=false` 由用户本人掌握；文档稳定后恢复。
6. 需要更重的约束时另开 issue/PR，不直接改写本 ADR 的决策；推翻时新增 ADR 并标记本 ADR 为 superseded。

## 后果

- 计划评审门禁与 ticket 链接成为合并的硬条件，不再依赖人肉 review。
- `feat/*` / `hotfix/*` PR 必须先有对应 issue；纯文档或规范变更使用 `chore/*` 或 `spec-delta/*`。
- agent 直推 main 仍未被平台强制阻止，现阶段依赖用户本人恢复 `enforce_admins`。

## Amendment（2026-11-25）

- 决策 3 的 `Closes` 覆盖范围由 ADR-0010 扩展：所有非豁免 PR 都需要恰好一行 `Closes`，并校验票元数据；`spec-delta/*` 仅免 `Closes`/issue 校验。
- 决策 2 的 plan-review 文件范围不变；计划通过记录见 ADR-0009。
